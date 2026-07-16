// Package podman is the container-runtime adapter: it implements the Runtime,
// ImageBuilder, and ImageResolver ports against the podman CLI. It is the only place
// that knows podman's argument syntax. AppRunArgs and the *Args builders are pure (no
// I/O) so launch plans can be inspected and dry-run; the rest exec podman.
//
// What it deliberately does NOT decide: the network. AppRunArgs splices in the
// netFlags it is handed by a NetEnforcer (adapters/netenforce), so swapping the egress
// mechanism never touches this file (docs/architecture.md section 5.3, section 13).
package podman

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

// Container-side fixed paths (refined alongside the real launch path in later
// milestones: theme env wiring, agent sockets).
const (
	ctrXDGRuntime = "/run/zinc"
	ctrThemeDir   = "/etc/zinc/theme"
)

// Runtime implements ports.Runtime against podman. It is stateless.
type Runtime struct{}

// Compile-time checks that this adapter satisfies the ports it claims.
var (
	_ ports.Runtime       = Runtime{}
	_ ports.ImageBuilder  = Builder{}
	_ ports.ImageResolver = Resolver{}
)

// TerminalLaunch wraps a `podman ...` argv in the configured terminal emulator so a
// CLI/TUI app (StartConditions.Terminal) appears in its own window. term is the
// emulator argv (e.g. ["foot"] or ["xterm","-e"]); it is run as `term... podman
// <runArgs...>`. It wraps a `run` argv (single-terminal apps) or an `exec` argv
// (multiterminal) alike.
//
// When hold is set the podman invocation is wrapped in the host shell so the window
// pauses after it exits - the user can read final output/errors before the window
// closes. This is emulator-agnostic on purpose: the emulator is user-configured
// (section 9.1), so we don't rely on an emulator-specific --hold flag.
func TerminalLaunch(term, runArgs []string, hold bool) []string {
	out := append([]string{}, term...)
	if !hold {
		out = append(out, "podman")
		return append(out, runArgs...)
	}
	// Each arg is single-quoted so a command argv can never break out of the script
	// (the install/validate layers already reject the worst metacharacters, but the
	// shell wrapper must be safe on its own). printf's \n are interpreted by printf.
	script := "podman " + shellJoin(runArgs) +
		`; status=$?; printf '\n[zinc] exited (status %s) - press Enter to close\n' "$status"; read _`
	return append(out, "sh", "-c", script)
}

// shellQuote wraps str in single quotes for safe interpolation into an `sh -c`
// script, escaping any embedded single quote as the standard '\” sequence. Used only
// by TerminalLaunch's hold wrapper.
func shellQuote(str string) string {
	return "'" + strings.ReplaceAll(str, "'", `'\''`) + "'"
}

// shellJoin single-quotes every arg and joins them with spaces.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for idx, arg := range args {
		quoted[idx] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

// HolderCmd is the main process of a multiterminal app's shared container: a no-op
// that blocks forever so the container outlives any single terminal. It runs under
// `--init` (see modeHolder): a bare `sleep` as PID 1 would ignore `podman stop` until
// the SIGKILL timeout; the injected init (catatonit) owns PID 1, handles SIGTERM, and
// tears down promptly. Needs `sleep` in the image.
func HolderCmd() []string { return []string{"sleep", "infinity"} }

// ExecArgs builds `podman exec -it <app> <cmd...>` - one interactive session into a
// running container. Each terminal of a multiterminal app is one of these (its cmd is
// the app's own command, or a shell), wrapped by TerminalLaunch.
func ExecArgs(app string, cmd []string) []string {
	out := []string{"exec", "-it", app}
	return append(out, cmd...)
}

// runMode selects the lifecycle flags and trailing command of a `podman run`.
type runMode int

const (
	modeForeground runMode = iota // plain `run --rm`
	modeBackground                // `run -d`
	modeTerminal                  // `run --rm -it` (single interactive terminal)
	modeHolder                    // `run -d --rm --init` + HolderCmd (multiterminal keep-alive)
)

// modeFor derives the run mode from a validated config. Multiterminal takes
// precedence: such an app's container is the holder, and its real command runs in each
// terminal via ExecArgs, not as the container's PID 1.
func modeFor(cfg schema.AppConfig) runMode {
	switch {
	case cfg.StartConditions.Multiterminal:
		return modeHolder
	case cfg.StartConditions.Terminal:
		return modeTerminal
	case cfg.StopConditions.Background:
		return modeBackground
	default:
		return modeForeground
	}
}

// AppRunArgs builds the app container's `podman run` argv. netFlags is the network
// attachment supplied by the NetEnforcer (e.g. ["--pod","app-pod"] or
// ["--network","none"]) and is spliced in after the least-privilege baseline; this
// adapter never decides the network itself. The trailing image is derived.RunImage
// (the derived image when ImageMeta.Install is set, else the base). Pure: no I/O.
func (Runtime) AppRunArgs(cfg schema.AppConfig, opt options.HostOptions, netFlags []string) ([]string, error) {
	home := opt.HomeDir
	if home == "" {
		home = "/root"
	}

	args := []string{"run"}
	mode := modeFor(cfg)
	// StopConditions.KeepAlive keeps the container after its entrypoint exits, so it is
	// not auto-removed (--rm dropped) for the non-detached modes.
	keepAlive := cfg.StopConditions.KeepAlive
	switch mode {
	case modeTerminal:
		// CLI/TUI app: needs an interactive TTY and runs in a spawned terminal window
		// (the shell wraps this argv with the emulator; see TerminalLaunch).
		if keepAlive {
			args = append(args, "-it")
		} else {
			args = append(args, "--rm", "-it")
		}
	case modeBackground:
		args = append(args, "-d")
	case modeHolder:
		// Multiterminal keep-alive: detached, no TTY, removed on stop (--rm), with
		// --init so `podman stop` is prompt (see HolderCmd). Its terminals attach via
		// ExecArgs.
		args = append(args, "-d", "--rm", "--init")
	default: // modeForeground
		if !keepAlive {
			args = append(args, "--rm")
		}
	}
	if cfg.StartConditions.Autorestart {
		// Restart only on failure - a clean exit (or a manual stop) is intentional (section 9.1).
		args = append(args, "--restart", "on-failure")
	}
	// Launch is hermetic: never fetch the image at run time (section 5.5). The image must
	// already be in local storage (a derived build, or resolved/pulled at save time); a
	// missing image fails fast instead of a surprise registry pull.
	args = append(args, "--pull", "never")
	args = append(args, "--name", cfg.AppNameID)

	// Least-privilege baseline (section 1, section 5.1): drop every capability and forbid privilege
	// escalation. Anything the app genuinely needs is re-added below from Capabilities.
	args = append(args, "--security-opt", "no-new-privileges", "--cap-drop", "all")

	// Network attachment is the enforcer's decision (section 5.3) - we only splice it in.
	args = append(args, netFlags...)

	// XDG_RUNTIME_DIR is exported once, and only when we actually mount a socket under
	// it (Wayland and/or Pipewire below). Exporting it unconditionally would point apps
	// at /run/zinc even when it's empty/absent in the container.
	runtimeDirExported := false
	exportRuntimeDir := func() {
		if !runtimeDirExported {
			args = append(args, "-e", "XDG_RUNTIME_DIR="+ctrXDGRuntime)
			runtimeDirExported = true
		}
	}

	// Display / Wayland (section 5.2).
	if opt.RuntimeDir != "" && opt.WaylandDisplay != "" {
		socket := filepath.Join(opt.RuntimeDir, opt.WaylandDisplay)
		args = append(args,
			"-v", socket+":"+filepath.Join(ctrXDGRuntime, opt.WaylandDisplay)+":ro",
			"-e", "WAYLAND_DISPLAY="+opt.WaylandDisplay,
		)
		exportRuntimeDir()
		if !cfg.DisplayMeta.DisableSecurityContext {
			args = append(args, "--label", "zinc.wayland=security-context")
		}
	}
	if !cfg.DisplayMeta.DisableGpuAccess {
		args = append(args, "--device", "/dev/dri")
	}

	// Theme bundle - one curated read-only directory (section 5.6).
	if cfg.HostTheme && opt.ThemeBundleDir != "" {
		args = append(args, "-v", opt.ThemeBundleDir+":"+ctrThemeDir+":ro")
	}

	// Audio (section 3 AudioMeta).
	if cfg.AudioMeta.Pipewire && opt.RuntimeDir != "" {
		pipewireSock := filepath.Join(opt.RuntimeDir, "pipewire-0")
		args = append(args, "-v", pipewireSock+":"+filepath.Join(ctrXDGRuntime, "pipewire-0")+":ro")
		exportRuntimeDir()
	}
	if cfg.AudioMeta.LegacyALSA {
		args = append(args, "--device", "/dev/snd")
	}

	// Host-mounted volumes (section 3 Volumes). Anonymous/size-limited volumes and Configs
	// (bundle-relative) are deferred; only explicit host bind mounts are wired here.
	for _, volume := range cfg.Volumes {
		if !volume.HostMounted || strings.TrimSpace(volume.HostMount) == "" {
			continue
		}
		mountOpts := "ro"
		if volume.Writable {
			mountOpts = "rw"
		}
		if volume.Executable {
			mountOpts += ",exec"
		} else {
			mountOpts += ",noexec"
		}
		args = append(args, "-v", volume.HostMount+":"+volume.InnerMount+":"+mountOpts)
	}

	// SSH/GPG keys (section 3 Keys) - RO file mounts into the container home.
	for _, key := range cfg.Keys {
		dir := ".ssh"
		if key.Type == schema.GPG {
			dir = ".gnupg"
		}
		args = append(args, "-v", key.Path+":"+filepath.Join(home, dir, filepath.Base(key.Path))+":ro")
	}

	// Extra capabilities (section 3 Capabilities).
	for _, capability := range cfg.Capabilities {
		args = append(args, "--cap-add", capability)
	}

	// Entrypoint override (exec form): replaces the image ENTRYPOINT. A holder runs
	// HolderCmd as PID 1 instead (the app's real command runs per-terminal via
	// ExecArgs), so it ignores the entrypoint.
	if mode != modeHolder {
		if entry := strings.TrimSpace(cfg.StartConditions.Entrypoint); entry != "" {
			args = append(args, "--entrypoint", entry)
		}
	}

	// Image, then (for a holder) the blocking command. A non-holder relies on
	// --entrypoint / the image default; there are no trailing args.
	args = append(args, derived.RunImage(cfg))
	if mode == modeHolder {
		args = append(args, HolderCmd()...)
	}
	return args, nil
}

// Lifecycle argv builders (section 9.1). Pure functions returning the arguments to pass to
// `podman` for the container named after the app.
func StopArgs(name string) []string    { return []string{"stop", name} }
func RestartArgs(name string) []string { return []string{"restart", name} }
func InspectArgs(name string) []string { return []string{"inspect", name} }

// LogsArgs builds `podman logs [-f] <name>`.
func LogsArgs(name string, follow bool) []string {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	return append(args, name)
}

// Exec runs one prepared command (pod create / nft lock / holder start), capturing
// output so a failure is reported with its podman error rather than silently. The
// command's Desc labels the error; the app layer adds the app name.
func (Runtime) Exec(cmd ports.Command) error {
	proc := exec.Command("podman", cmd.Args...)
	if cmd.Stdin != "" {
		proc.Stdin = strings.NewReader(cmd.Stdin)
	}
	if out, err := proc.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", cmd.Desc, strings.TrimSpace(string(out)))
	}
	return nil
}

// StartApp starts the app container detached from the caller (Setsid) so it outlives a
// launcher that exits right after it. A terminal app is wrapped in the configured
// emulator; a GUI app renders through the Wayland socket. It returns once the process
// is forked, before `podman run` succeeds; if the app then exits with an error, onFail
// runs from the reaping goroutine so a post-fork failure can tear down the prepared
// (still-filtered) pod/netns instead of leaking it.
func (Runtime) StartApp(cfg schema.AppConfig, opt options.HostOptions, runArgs []string, onFail func()) error {
	proc, err := appCmd(cfg, opt, runArgs)
	if err != nil {
		return err
	}
	if err := proc.Start(); err != nil {
		return fmt.Errorf("launch %s: %w", cfg.AppNameID, err)
	}
	go func() {
		// reap if the caller (long-lived TUI) outlives the app; a non-nil Wait means the
		// app died post-fork, so tear down what Prepare left in place.
		if err := proc.Wait(); err != nil && onFail != nil {
			onFail()
		}
	}()
	return nil
}

// appCmd builds the detached command for the app container: a plain `podman run` for a
// GUI app, or the configured terminal emulator wrapping it for a terminal app. Setsid
// puts it in its own session so closing the launcher doesn't take the app down. Split
// out from StartApp so the argv/Setsid wiring is unit-testable.
func appCmd(cfg schema.AppConfig, opt options.HostOptions, runArgs []string) (*exec.Cmd, error) {
	var proc *exec.Cmd
	if cfg.StartConditions.Terminal {
		if len(opt.Terminal) == 0 {
			return nil, fmt.Errorf("%s: terminal app but no terminal emulator configured (set ZINC_TERMINAL)", cfg.AppNameID)
		}
		wrap := TerminalLaunch(opt.Terminal, runArgs, false)
		proc = exec.Command(wrap[0], wrap[1:]...)
	} else {
		proc = exec.Command("podman", runArgs...) // stdio nil → /dev/null; GUI renders via Wayland
	}
	proc.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return proc, nil
}

// OpenSession opens one terminal of a multiterminal app: the configured emulator
// wrapping a `podman exec -it` into the holder, running cmd. It blocks until the
// terminal window closes.
func (Runtime) OpenSession(app string, cmd []string, opt options.HostOptions, hold bool) error {
	argv := TerminalLaunch(opt.Terminal, ExecArgs(app, cmd), hold)
	return exec.Command(argv[0], argv[1:]...).Run()
}

// Exists reports whether a container with this name exists (running or not).
func (Runtime) Exists(name string) bool {
	return exec.Command("podman", "container", "exists", name).Run() == nil
}

// Do runs a user-facing podman command (stop/restart/inspect/logs) with the host's
// stdio attached, so output and follow-mode stream straight to the terminal.
func (Runtime) Do(args []string) error {
	pc := exec.Command("podman", args...)
	pc.Stdin, pc.Stdout, pc.Stderr = os.Stdin, os.Stdout, os.Stderr
	return pc.Run()
}

// Running returns the set of container names podman currently reports as running. A
// query failure yields an empty set (not an error) so the list view degrades to
// "nothing running" rather than failing to load.
func (Runtime) Running() (map[string]bool, error) {
	set := map[string]bool{}
	out, err := exec.Command("podman", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return set, nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set, nil
}

// Logs returns the last tail lines of a container's logs. podman may exit nonzero
// (e.g. the container never ran) but still print useful output, so both are returned
// for the caller to format.
func (Runtime) Logs(name string, tail int) (string, error) {
	out, err := exec.Command("podman", "logs", "--tail", strconv.Itoa(tail), name).CombinedOutput()
	return string(out), err
}

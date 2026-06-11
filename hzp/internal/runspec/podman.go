// Package runspec turns a validated config.AppConfig into the argument vector for
// a container runtime. BuildArgs is a pure function: it performs no I/O and reads
// the host environment only through the explicit Options it is given, so the
// translation is fully testable.
//
// What it does NOT do (deliberately, see docs/architecture.md):
//   - It does not enforce the pasta egress allowlist; that is an nftables ruleset
//     applied inside the container's own netns after start (§5.3).
//   - It does not generate the theme bundle or the theme env vars; the home-manager
//     module owns those (§5.6/§9.3). Here we only mount the bundle directory.
//   - Agent-socket wiring and 0600 enforcement for [keys] land in a later milestone;
//     for now key files are mounted read-only.
package runspec

import (
	"fmt"
	"path/filepath"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
)

// Container-side fixed paths. Placeholders for M0 — refined alongside the real
// launch path (theme env wiring, agent sockets) in later milestones.
const (
	ctrXDGRuntime = "/run/hyprzinc"
	ctrThemeDir   = "/etc/hyprzinc/theme"
)

// Options carries the host-side values the builder needs, passed in explicitly so
// BuildArgs stays pure. Empty fields disable the corresponding wiring.
type Options struct {
	RuntimeDir     string // host XDG_RUNTIME_DIR (wayland/pipewire sockets)
	WaylandDisplay string // host WAYLAND_DISPLAY, e.g. "wayland-1"
	ThemeBundleDir string // host path to the generated curated theme bundle (§5.6)
	HomeDir        string // container user's home (key placement); defaults to /root
}

// BuildArgs returns the arguments to pass to `podman` (starting with "run") for
// the given app. cfg is assumed to have passed config.Validate.
func BuildArgs(cfg config.AppConfig, opt Options) ([]string, error) {
	home := opt.HomeDir
	if home == "" {
		home = "/root"
	}

	args := []string{"run"}
	if cfg.App.Background {
		args = append(args, "-d")
	} else {
		args = append(args, "--rm")
	}
	args = append(args, "--name", cfg.App.Name)

	// Least-privilege baseline (§1 "nothing unless explicitly granted", §5.1):
	// drop every capability and forbid privilege escalation. Anything the app
	// genuinely needs is re-added below from [capabilities].extra — and only that.
	args = append(args, "--security-opt", "no-new-privileges", "--cap-drop", "all")

	// Network (§5.3, §6.4).
	switch cfg.Network.Mode {
	case config.NetworkNone:
		args = append(args, "--network", "none")
	case config.NetworkPasta:
		args = append(args, "--network", "pasta")
	case config.NetworkContainer:
		args = append(args, "--network", "container:"+cfg.Network.Target)
	default:
		return nil, fmt.Errorf("runspec: unknown network mode %q", cfg.Network.Mode)
	}

	// XDG_RUNTIME_DIR is exported once, and only when we actually mount a socket
	// under it (Wayland and/or Pipewire below). Exporting it unconditionally would
	// point apps at /run/hyprzinc even when it's empty/absent in the container,
	// breaking libraries that expect a real, 0700 runtime dir (dbus, etc.).
	runtimeDirExported := false
	exportRuntimeDir := func() {
		if !runtimeDirExported {
			args = append(args, "-e", "XDG_RUNTIME_DIR="+ctrXDGRuntime)
			runtimeDirExported = true
		}
	}

	// Display / Wayland (§5.2).
	if opt.RuntimeDir != "" && opt.WaylandDisplay != "" {
		socket := filepath.Join(opt.RuntimeDir, opt.WaylandDisplay)
		args = append(args,
			"-v", socket+":"+filepath.Join(ctrXDGRuntime, opt.WaylandDisplay)+":ro",
			"-e", "WAYLAND_DISPLAY="+opt.WaylandDisplay,
		)
		exportRuntimeDir()
		// security-context is negotiated with the compositor at connect time; we
		// only label intent here. Real isolation is toolkit-dependent (§5.2).
		if cfg.Display.Wayland == config.WaylandSecurityContext {
			args = append(args, "--label", "hyprzinc.wayland=security-context")
		}
	}
	if cfg.Display.GPU {
		args = append(args, "--device", "/dev/dri")
	}

	// Theme bundle — one curated read-only directory (§5.6).
	if cfg.Theme.Mode == config.ThemeHost && opt.ThemeBundleDir != "" {
		args = append(args, "-v", opt.ThemeBundleDir+":"+ctrThemeDir+":ro")
	}

	// Audio (§3 [audio]).
	if cfg.Audio.Pipewire && opt.RuntimeDir != "" {
		pipewireSock := filepath.Join(opt.RuntimeDir, "pipewire-0")
		args = append(args, "-v", pipewireSock+":"+filepath.Join(ctrXDGRuntime, "pipewire-0")+":ro")
		exportRuntimeDir()
	}
	if cfg.Audio.LegacyALSA {
		args = append(args, "--device", "/dev/snd")
	}

	// General mounts (§3 [[mounts]]).
	for _, mount := range cfg.Mounts {
		args = append(args, "-v", mount.Host+":"+mount.Container+":"+mount.Mode)
	}

	// SSH/GPG keys (§3 [keys]) — RO file mounts for now.
	for _, keyPath := range cfg.Keys.SSH {
		args = append(args, "-v", keyPath+":"+filepath.Join(home, ".ssh", filepath.Base(keyPath))+":ro")
	}
	for _, keyPath := range cfg.Keys.GPG {
		args = append(args, "-v", keyPath+":"+filepath.Join(home, ".gnupg", filepath.Base(keyPath))+":ro")
	}

	// Extra capabilities (§3 [capabilities]).
	for _, capability := range cfg.Capabilities.Extra {
		args = append(args, "--cap-add", capability)
	}

	// Image last (§5.5).
	args = append(args, cfg.App.Image)
	return args, nil
}

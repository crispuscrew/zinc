package podman

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func baseOpts() options.HostOptions {
	return options.HostOptions{
		RuntimeDir:     "/run/user/1000",
		WaylandDisplay: "wayland-1",
		ThemeBundleDir: "/home/user/.local/share/zinc/theme-bundle",
		HomeDir:        "/root",
	}
}

// netNone is the network attachment an unfiltered app hands AppRunArgs; the podman
// adapter only splices it in (it no longer decides the network itself).
func netNone() []string { return []string{"--network", "none"} }

func appArgs(t *testing.T, cfg schema.AppConfig, opt options.HostOptions, netFlags []string) []string {
	t.Helper()
	got, err := Runtime{}.AppRunArgs(cfg, opt, netFlags)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A strict, no-network app: exact argv, so the least-privilege baseline, hermetic
// --pull never, security-context label, and the Wayland/theme wiring are all pinned.
func TestAppRunArgs_StrictNone(t *testing.T) {
	cfg := schema.AppConfig{
		AppNameID:   "firefox",
		ImageMeta:   schema.ImageMeta{Image: "docker.io/library/firefox@sha256:abc"},
		DisplayMeta: schema.DisplayMeta{DisableGpuAccess: true}, // security-context on, no GPU
		HostTheme:   true,
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	want := []string{
		"run", "--rm", "--pull", "never", "--name", "firefox",
		"--security-opt", "no-new-privileges", "--cap-drop", "all",
		"--network", "none",
		"-v", "/run/user/1000/wayland-1:/run/zinc/wayland-1:ro",
		"-e", "WAYLAND_DISPLAY=wayland-1",
		"-e", "XDG_RUNTIME_DIR=/run/zinc",
		"--label", "zinc.wayland=security-context",
		"-v", "/home/user/.local/share/zinc/theme-bundle:/etc/zinc/theme:ro",
		"docker.io/library/firefox@sha256:abc",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("argv mismatch:\n got: %v\nwant: %v", got, want)
	}
}

// A background app with GPU, a host bind mount, pipewire, and an extra cap: the
// enforcer's netFlags are spliced verbatim and every wiring is present.
func TestAppRunArgs_BackgroundGPUMountCap(t *testing.T) {
	cfg := schema.AppConfig{
		AppNameID:      "work-app",
		ImageMeta:      schema.ImageMeta{Image: "localhost/zinc-go-dev:latest"},
		StopConditions: schema.StopConditions{Background: true},
		DisplayMeta:    schema.DisplayMeta{DisableSecurityContext: true}, // passthrough; GPU on (default)
		Volumes:        []schema.Volume{{InnerMount: "/work", HostMounted: true, HostMount: "/home/user/code", Writable: true}},
		AudioMeta:      schema.AudioMeta{Pipewire: true},
		Capabilities:   []string{"NET_RAW"},
	}
	got := appArgs(t, cfg, baseOpts(), []string{"--network", "container:vpn"})
	assertContainsSeq(t, got, "--network", "container:vpn") // spliced verbatim
	assertContainsSeq(t, got, "--cap-drop", "all")          // least-privilege baseline
	assertContainsSeq(t, got, "--cap-add", "NET_RAW")       // explicit grant on top
	assertContains(t, got, "-d")                            // background
	assertContains(t, got, "/dev/dri")                      // gpu on (opt-out default)
	mustNotContain(t, got, "/dev/snd")                      // legacy_alsa was false
	assertContainsSeq(t, got, "-v", "/home/user/code:/work:rw,noexec")
	mustNotContain(t, got, "/etc/zinc/theme") // HostTheme false → no bundle mount
	assertContainsSeq(t, got, "-v", "/run/user/1000/pipewire-0:/run/zinc/pipewire-0:ro")
	if got[len(got)-1] != "localhost/zinc-go-dev:latest" {
		t.Fatalf("image must be last arg, got %q", got[len(got)-1])
	}
}

func TestAppRunArgs_PipewireWithoutWayland(t *testing.T) {
	cfg := schema.AppConfig{
		AppNameID: "mpd",
		ImageMeta: schema.ImageMeta{Image: "img@sha256:abc"},
		AudioMeta: schema.AudioMeta{Pipewire: true},
	}
	opt := baseOpts()
	opt.WaylandDisplay = "" // headless: no Wayland socket wired
	got := appArgs(t, cfg, opt, netNone())
	assertContainsSeq(t, got, "-v", "/run/user/1000/pipewire-0:/run/zinc/pipewire-0:ro")
	assertContainsSeq(t, got, "-e", "XDG_RUNTIME_DIR=/run/zinc")
}

func TestAppRunArgs_NoRuntimeDirWithoutSockets(t *testing.T) {
	cfg := schema.AppConfig{
		AppNameID:   "tool",
		ImageMeta:   schema.ImageMeta{Image: "img@sha256:abc"},
		DisplayMeta: schema.DisplayMeta{DisableGpuAccess: true},
	}
	opt := baseOpts()
	opt.WaylandDisplay = "" // no wayland, no audio → runtime dir stays empty
	got := appArgs(t, cfg, opt, netNone())
	mustNotContain(t, got, "XDG_RUNTIME_DIR=/run/zinc")
	assertContainsSeq(t, got, "--security-opt", "no-new-privileges")
	assertContainsSeq(t, got, "--cap-drop", "all")
}

func TestAppRunArgs_Terminal(t *testing.T) {
	cfg := schema.AppConfig{
		AppNameID:       "shell",
		ImageMeta:       schema.ImageMeta{Image: "docker.io/library/alpine@sha256:abc"},
		StartConditions: schema.StartConditions{Terminal: true},
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	if got[0] != "run" {
		t.Fatalf("argv must start with run, got %v", got)
	}
	assertContainsSeq(t, got, "--rm", "-it") // interactive TTY for a CLI/TUI app
	mustNotContain(t, got, "-d")             // terminal apps are never detached/background
}

// The entrypoint overrides the image ENTRYPOINT via --entrypoint (exec form); the
// image is the last arg with no trailing command.
func TestAppRunArgs_Entrypoint(t *testing.T) {
	cfg := schema.AppConfig{
		AppNameID:       "shell",
		ImageMeta:       schema.ImageMeta{Image: "img@sha256:abc"},
		StartConditions: schema.StartConditions{Entrypoint: "htop"},
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	assertContainsSeq(t, got, "--entrypoint", "htop")
	if last := got[len(got)-1]; last != "img@sha256:abc" {
		t.Fatalf("image must be the last arg (no trailing cmd with --entrypoint), got %q", last)
	}
}

// KeepAlive keeps the container after its entrypoint exits, so --rm is dropped.
func TestAppRunArgs_KeepAlive(t *testing.T) {
	cfg := schema.AppConfig{
		AppNameID:      "job",
		ImageMeta:      schema.ImageMeta{Image: "img@sha256:abc"},
		StopConditions: schema.StopConditions{KeepAlive: true},
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	mustNotContain(t, got, "--rm")
}

func TestAppRunArgs_Holder(t *testing.T) {
	// A multiterminal app's container is a detached holder: -d --rm, no -it, and
	// HolderCmd as PID 1 - the app's own command runs per-terminal via ExecArgs.
	cfg := schema.AppConfig{
		AppNameID: "dev",
		ImageMeta: schema.ImageMeta{Image: "docker.io/library/alpine@sha256:abc"},
		StartConditions: schema.StartConditions{
			Terminal: true, Multiterminal: true, Entrypoint: "htop",
		},
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	assertContainsSeq(t, got, "-d", "--rm")
	assertContains(t, got, "--init")       // prompt `podman stop` (PID-1 signal semantics)
	mustNotContain(t, got, "-it")          // holder has no TTY
	mustNotContain(t, got, "--entrypoint") // holder ignores the app entrypoint
	wantTail := append([]string{"docker.io/library/alpine@sha256:abc"}, HolderCmd()...)
	if tail := got[len(got)-len(wantTail):]; !slices.Equal(tail, wantTail) {
		t.Fatalf("holder cmd must follow the image, got tail %v want %v", tail, wantTail)
	}
}

// --- derived images (install) ---

func installCfg(install ...string) schema.AppConfig {
	return schema.AppConfig{
		AppNameID: "hollywood",
		ImageMeta: schema.ImageMeta{
			Image:   "docker.io/library/debian@sha256:abc",
			Install: install,
		},
	}
}

// With ImageMeta.Install set, the container must run the locally built derived image,
// not the pinned base - the base is only the FROM of that build.
func TestAppRunArgs_InstallRunsDerivedImage(t *testing.T) {
	got := appArgs(t, installCfg("apt-get install -y hollywood"), baseOpts(), netNone())
	if last := got[len(got)-1]; last != "zinc/app-hollywood:local" {
		t.Fatalf("install app must run the derived image, got last arg %q", last)
	}
	mustNotContain(t, got, "docker.io/library/debian@sha256:abc") // base is only the FROM
}

func TestAppRunArgs_InstallHolder(t *testing.T) {
	cfg := installCfg("apk add --no-cache htop")
	cfg.StartConditions.Terminal, cfg.StartConditions.Multiterminal = true, true
	cfg.StartConditions.Entrypoint = "htop"
	got := appArgs(t, cfg, baseOpts(), netNone())
	wantTail := append([]string{"zinc/app-hollywood:local"}, HolderCmd()...)
	if tail := got[len(got)-len(wantTail):]; !slices.Equal(tail, wantTail) {
		t.Fatalf("holder install app must run the derived image, got tail %v want %v", tail, wantTail)
	}
}

func TestImageBuildArgs(t *testing.T) {
	cfg := installCfg("apk add --no-cache sl")
	got := ImageBuildArgs(cfg)
	if got[0] != "build" || got[len(got)-1] != "-" {
		t.Fatalf("want `build ... -` (Containerfile on stdin), got %v", got)
	}
	assertContainsSeq(t, got, "-t", "zinc/app-hollywood:local")
	assertContainsSeq(t, got, "--label", "zinc.build="+derived.BuildFingerprint(cfg))
}

// --- pure builders + detached command wiring ---

func TestExecArgs(t *testing.T) {
	if got := ExecArgs("dev", []string{"htop", "--tree"}); !slices.Equal(
		got, []string{"exec", "-it", "dev", "htop", "--tree"}) {
		t.Fatalf("exec argv mismatch: %v", got)
	}
	if got := ExecArgs("dev", []string{"/bin/sh"}); !slices.Equal(
		got, []string{"exec", "-it", "dev", "/bin/sh"}) {
		t.Fatalf("shell exec argv mismatch: %v", got)
	}
}

func TestTerminalLaunch(t *testing.T) {
	got := TerminalLaunch([]string{"xterm", "-e"}, []string{"run", "--rm", "-it", "alpine"}, false)
	want := []string{"xterm", "-e", "podman", "run", "--rm", "-it", "alpine"}
	if !slices.Equal(got, want) {
		t.Fatalf("terminal wrap mismatch:\n got %v\nwant %v", got, want)
	}
}

// With hold the podman argv is wrapped in `sh -c` so the window pauses after the
// command exits; the argv must be single-quoted (no break-out) and the script must
// block on input at the end.
func TestTerminalLaunchHold(t *testing.T) {
	got := TerminalLaunch([]string{"foot"}, []string{"run", "--rm", "-it", "alpine"}, true)
	if len(got) != 4 || got[0] != "foot" || got[1] != "sh" || got[2] != "-c" {
		t.Fatalf("hold wrap should be `foot sh -c <script>`, got %v", got)
	}
	script := got[3]
	if !strings.Contains(script, "podman 'run' '--rm' '-it' 'alpine'") {
		t.Fatalf("script missing single-quoted podman argv: %q", script)
	}
	if !strings.Contains(script, "read _") {
		t.Fatalf("script should pause on exit: %q", script)
	}
}

// shellQuote must neutralise an embedded single quote so a crafted command argv cannot
// escape the hold wrapper.
func TestShellQuoteEscapesSingleQuote(t *testing.T) {
	if got, want := shellQuote(`a'b`), `'a'\''b'`; got != want {
		t.Fatalf("shellQuote(%q) = %q, want %q", `a'b`, got, want)
	}
}

func TestLifecycleArgs(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"stop", StopArgs("firefox"), []string{"stop", "firefox"}},
		{"restart", RestartArgs("firefox"), []string{"restart", "firefox"}},
		{"inspect", InspectArgs("firefox"), []string{"inspect", "firefox"}},
		{"logs", LogsArgs("firefox", false), []string{"logs", "firefox"}},
		{"logs follow", LogsArgs("firefox", true), []string{"logs", "-f", "firefox"}},
	}
	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			if !slices.Equal(tcase.got, tcase.want) {
				t.Fatalf("got %v, want %v", tcase.got, tcase.want)
			}
		})
	}
}

func validCfg() schema.AppConfig {
	return schema.AppConfig{
		AppNameID: "demo",
		ImageMeta: schema.ImageMeta{Image: "docker.io/library/demo@sha256:abc"},
	}
}

func TestAppCmd_GUI(t *testing.T) {
	pc, err := appCmd(validCfg(), options.HostOptions{}, []string{"run", "--rm", "img"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"podman", "run", "--rm", "img"}; !slices.Equal(pc.Args, want) {
		t.Fatalf("gui app argv: got %v want %v", pc.Args, want)
	}
	if pc.SysProcAttr == nil || !pc.SysProcAttr.Setsid {
		t.Fatal("launched app must be detached into its own session (Setsid) so it survives the launcher")
	}
}

func TestAppCmd_Terminal(t *testing.T) {
	cfg := validCfg()
	cfg.StartConditions.Terminal = true
	pc, err := appCmd(cfg, options.HostOptions{Terminal: []string{"foot"}}, []string{"run", "--rm", "-it", "img"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"foot", "podman", "run", "--rm", "-it", "img"}; !slices.Equal(pc.Args, want) {
		t.Fatalf("terminal app argv: got %v want %v", pc.Args, want)
	}
}

func TestAppCmd_TerminalNoEmulator(t *testing.T) {
	cfg := validCfg()
	cfg.StartConditions.Terminal = true
	if _, err := appCmd(cfg, options.HostOptions{}, []string{"run"}); err == nil {
		t.Fatal("a terminal app with no configured emulator must error, not launch blind")
	}
}

// --- test helpers ---

func assertContains(t *testing.T, args []string, want string) {
	t.Helper()
	if !slices.Contains(args, want) {
		t.Fatalf("expected args to contain %q; got %v", want, args)
	}
}

func mustNotContain(t *testing.T, args []string, bad string) {
	t.Helper()
	if slices.Contains(args, bad) {
		t.Fatalf("did not expect args to contain %q; got %v", bad, args)
	}
}

// assertContainsSeq checks that first and second appear adjacent and in order.
func assertContainsSeq(t *testing.T, args []string, first, second string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return
		}
	}
	t.Fatalf("expected adjacent %q %q in %v", first, second, args)
}

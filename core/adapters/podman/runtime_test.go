package podman

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/hyprzinc/core/domain"
)

func baseOpts() domain.HostOptions {
	return domain.HostOptions{
		RuntimeDir:     "/run/user/1000",
		WaylandDisplay: "wayland-1",
		ThemeBundleDir: "/home/user/.local/share/hyprzinc/theme-bundle",
		HomeDir:        "/root",
	}
}

// netNone is the network attachment a None enforcer hands AppRunArgs; the podman
// adapter only splices it in (it no longer decides the network itself).
func netNone() []string { return []string{"--network", "none"} }

func appArgs(t *testing.T, cfg domain.AppConfig, opt domain.HostOptions, netFlags []string) []string {
	t.Helper()
	got, err := Runtime{}.AppRunArgs(cfg, opt, netFlags)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAppRunArgs_StrictNone(t *testing.T) {
	cfg := domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App:           domain.App{Name: "firefox", Image: "docker.io/library/firefox@sha256:abc"},
		Display:       domain.Display{Wayland: domain.WaylandSecurityContext},
		Network:       domain.Network{Mode: domain.NetworkNone},
		Theme:         domain.Theme{Mode: domain.ThemeHost},
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	want := []string{
		"run", "--rm", "--name", "firefox",
		"--security-opt", "no-new-privileges", "--cap-drop", "all",
		"--network", "none",
		"-v", "/run/user/1000/wayland-1:/run/hyprzinc/wayland-1:ro",
		"-e", "WAYLAND_DISPLAY=wayland-1",
		"-e", "XDG_RUNTIME_DIR=/run/hyprzinc",
		"--label", "hyprzinc.wayland=security-context",
		"-v", "/home/user/.local/share/hyprzinc/theme-bundle:/etc/hyprzinc/theme:ro",
		"docker.io/library/firefox@sha256:abc",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("argv mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestAppRunArgs_ContainerGPUMountCap(t *testing.T) {
	cfg := domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App:           domain.App{Name: "work-app", Image: "hyprzinc/trusted-go-dev:latest", Background: true},
		Display:       domain.Display{Wayland: domain.WaylandPassthrough, GPU: true},
		Network:       domain.Network{Mode: domain.NetworkContainer, Target: "vpn-container"},
		Theme:         domain.Theme{Mode: domain.ThemeNone},
		Mounts:        []domain.Mount{{Host: "/home/user/code", Container: "/work", Mode: domain.MountRW}},
		Audio:         domain.Audio{Pipewire: true},
		Capabilities:  domain.Capabilities{Extra: []string{"NET_RAW"}},
	}
	got := appArgs(t, cfg, baseOpts(), []string{"--network", "container:vpn-container"})
	assertContainsSeq(t, got, "--network", "container:vpn-container")
	assertContainsSeq(t, got, "--cap-drop", "all")    // least-privilege baseline
	assertContainsSeq(t, got, "--cap-add", "NET_RAW") // explicit grant on top
	assertContains(t, got, "-d")                      // background
	assertContains(t, got, "/dev/dri")                // gpu device
	mustNotContain(t, got, "/dev/snd")                // legacy_alsa was false
	assertContainsSeq(t, got, "-v", "/home/user/code:/work:rw")
	mustNotContain(t, got, "/etc/hyprzinc/theme") // theme.mode=none → no bundle mount
	assertContainsSeq(t, got, "-v", "/run/user/1000/pipewire-0:/run/hyprzinc/pipewire-0:ro")
	if got[len(got)-1] != "hyprzinc/trusted-go-dev:latest" {
		t.Fatalf("image must be last arg, got %q", got[len(got)-1])
	}
}

func TestAppRunArgs_PipewireWithoutWayland(t *testing.T) {
	cfg := domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App:           domain.App{Name: "mpd", Image: "img@sha256:abc"},
		Display:       domain.Display{Wayland: domain.WaylandSecurityContext},
		Network:       domain.Network{Mode: domain.NetworkNone},
		Theme:         domain.Theme{Mode: domain.ThemeNone},
		Audio:         domain.Audio{Pipewire: true},
	}
	opt := baseOpts()
	opt.WaylandDisplay = "" // headless: no Wayland socket wired
	got := appArgs(t, cfg, opt, netNone())
	assertContainsSeq(t, got, "-v", "/run/user/1000/pipewire-0:/run/hyprzinc/pipewire-0:ro")
	assertContainsSeq(t, got, "-e", "XDG_RUNTIME_DIR=/run/hyprzinc")
}

func TestAppRunArgs_NoRuntimeDirWithoutSockets(t *testing.T) {
	cfg := domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App:           domain.App{Name: "tool", Image: "img@sha256:abc"},
		Display:       domain.Display{Wayland: domain.WaylandSecurityContext},
		Network:       domain.Network{Mode: domain.NetworkNone},
		Theme:         domain.Theme{Mode: domain.ThemeNone},
	}
	opt := baseOpts()
	opt.WaylandDisplay = "" // no wayland, no audio → runtime dir stays empty
	got := appArgs(t, cfg, opt, netNone())
	mustNotContain(t, got, "XDG_RUNTIME_DIR=/run/hyprzinc")
	assertContainsSeq(t, got, "--security-opt", "no-new-privileges")
	assertContainsSeq(t, got, "--cap-drop", "all")
}

func TestAppRunArgs_Terminal(t *testing.T) {
	cfg := domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App:           domain.App{Name: "shell", Image: "docker.io/library/alpine@sha256:abc", Terminal: true},
		Display:       domain.Display{Wayland: domain.WaylandSecurityContext},
		Network:       domain.Network{Mode: domain.NetworkNone},
		Theme:         domain.Theme{Mode: domain.ThemeNone},
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	if got[0] != "run" {
		t.Fatalf("argv must start with run, got %v", got)
	}
	assertContainsSeq(t, got, "--rm", "-it") // interactive TTY for a CLI/TUI app
	mustNotContain(t, got, "-d")             // terminal apps are never detached/background
}

func TestAppRunArgs_Command(t *testing.T) {
	cfg := domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App:           domain.App{Name: "shell", Image: "img@sha256:abc", Command: []string{"htop", "--tree"}},
		Display:       domain.Display{Wayland: domain.WaylandSecurityContext},
		Network:       domain.Network{Mode: domain.NetworkNone},
		Theme:         domain.Theme{Mode: domain.ThemeNone},
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	if tail := got[len(got)-3:]; !slices.Equal(tail, []string{"img@sha256:abc", "htop", "--tree"}) {
		t.Fatalf("command argv must come right after the image, got tail %v", tail)
	}
}

func TestAppRunArgs_Holder(t *testing.T) {
	// A multiterminal app's container is a detached holder: -d --rm, no -it, and
	// HolderCmd as PID 1 — the app's own command runs per-terminal via ExecArgs.
	cfg := domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App: domain.App{
			Name: "dev", Image: "docker.io/library/alpine@sha256:abc",
			Terminal: true, Multiterminal: true, Command: []string{"htop"},
		},
		Display: domain.Display{Wayland: domain.WaylandSecurityContext},
		Network: domain.Network{Mode: domain.NetworkNone},
		Theme:   domain.Theme{Mode: domain.ThemeNone},
	}
	got := appArgs(t, cfg, baseOpts(), netNone())
	assertContainsSeq(t, got, "-d", "--rm")
	assertContains(t, got, "--init") // prompt `podman stop` (PID-1 signal semantics)
	mustNotContain(t, got, "-it")    // holder has no TTY
	mustNotContain(t, got, "htop")   // the app command does NOT run as PID 1
	wantTail := append([]string{"docker.io/library/alpine@sha256:abc"}, HolderCmd()...)
	if tail := got[len(got)-len(wantTail):]; !slices.Equal(tail, wantTail) {
		t.Fatalf("holder cmd must follow the image, got tail %v want %v", tail, wantTail)
	}
}

// --- derived images (install) ---

func installCfg(install string) domain.AppConfig {
	return domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App: domain.App{
			Name:    "hollywood",
			Image:   "docker.io/library/debian@sha256:abc",
			Install: install,
		},
		Display: domain.Display{Wayland: domain.WaylandSecurityContext},
		Network: domain.Network{Mode: domain.NetworkNone},
		Theme:   domain.Theme{Mode: domain.ThemeNone},
	}
}

// With app.install set, the container must run the locally built derived image, not
// the pinned base — the base is only the FROM of that build.
func TestAppRunArgs_InstallRunsDerivedImage(t *testing.T) {
	got := appArgs(t, installCfg("apt-get install -y hollywood"), baseOpts(), netNone())
	if last := got[len(got)-1]; last != "hyprzinc/app-hollywood:local" {
		t.Fatalf("install app must run the derived image, got last arg %q", last)
	}
	mustNotContain(t, got, "docker.io/library/debian@sha256:abc") // base is only the FROM
}

func TestAppRunArgs_InstallHolder(t *testing.T) {
	cfg := installCfg("apk add --no-cache htop")
	cfg.App.Terminal, cfg.App.Multiterminal = true, true
	cfg.App.Command = []string{"htop"}
	got := appArgs(t, cfg, baseOpts(), netNone())
	wantTail := append([]string{"hyprzinc/app-hollywood:local"}, HolderCmd()...)
	if tail := got[len(got)-len(wantTail):]; !slices.Equal(tail, wantTail) {
		t.Fatalf("holder install app must run the derived image, got tail %v want %v", tail, wantTail)
	}
}

func TestImageBuildArgs(t *testing.T) {
	cfg := installCfg("apk add --no-cache sl")
	got := ImageBuildArgs(cfg)
	if got[0] != "build" || got[len(got)-1] != "-" {
		t.Fatalf("want `build … -` (Containerfile on stdin), got %v", got)
	}
	assertContainsSeq(t, got, "-t", "hyprzinc/app-hollywood:local")
	assertContainsSeq(t, got, "--label", "hyprzinc.build="+domain.BuildFingerprint(cfg))
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

// With keep_open the podman argv is wrapped in `sh -c` so the window pauses after
// the command exits; the argv must be single-quoted (no break-out) and the script
// must block on input at the end.
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

// shellQuote must neutralise an embedded single quote so a crafted command argv
// cannot escape the keep_open wrapper.
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

func validCfg() domain.AppConfig {
	cfg, _ := domain.DefaultsFor(domain.PresetStrict)
	cfg.App.Name = "demo"
	cfg.App.Image = "docker.io/library/demo@sha256:abc"
	return cfg
}

func TestAppCmd_GUI(t *testing.T) {
	pc, err := appCmd(validCfg(), domain.HostOptions{}, []string{"run", "--rm", "img"})
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
	c := validCfg()
	c.App.Terminal = true
	pc, err := appCmd(c, domain.HostOptions{Terminal: []string{"foot"}}, []string{"run", "--rm", "-it", "img"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"foot", "podman", "run", "--rm", "-it", "img"}; !slices.Equal(pc.Args, want) {
		t.Fatalf("terminal app argv: got %v want %v", pc.Args, want)
	}
}

func TestAppCmd_TerminalNoEmulator(t *testing.T) {
	c := validCfg()
	c.App.Terminal = true
	if _, err := appCmd(c, domain.HostOptions{}, []string{"run"}); err == nil {
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

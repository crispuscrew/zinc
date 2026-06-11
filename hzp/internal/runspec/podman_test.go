package runspec

import (
	"slices"
	"testing"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
)

func baseOpts() Options {
	return Options{
		RuntimeDir:     "/run/user/1000",
		WaylandDisplay: "wayland-1",
		ThemeBundleDir: "/home/user/.local/share/hyprzinc/theme-bundle",
		HomeDir:        "/root",
	}
}

func TestBuildArgs_StrictNone(t *testing.T) {
	cfg := config.AppConfig{
		SchemaVersion: config.SchemaVersion,
		App:           config.App{Name: "firefox", Image: "docker.io/library/firefox@sha256:abc"},
		Display:       config.Display{Wayland: config.WaylandSecurityContext},
		Network:       config.Network{Mode: config.NetworkNone},
		Theme:         config.Theme{Mode: config.ThemeHost},
	}
	got, err := BuildArgs(cfg, baseOpts())
	if err != nil {
		t.Fatal(err)
	}
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

func TestBuildArgs_ContainerGPUMountCap(t *testing.T) {
	cfg := config.AppConfig{
		SchemaVersion: config.SchemaVersion,
		App:           config.App{Name: "work-app", Image: "hyprzinc/trusted-go-dev:latest", Background: true},
		Display:       config.Display{Wayland: config.WaylandPassthrough, GPU: true},
		Network:       config.Network{Mode: config.NetworkContainer, Target: "vpn-container"},
		Theme:         config.Theme{Mode: config.ThemeNone},
		Mounts:        []config.Mount{{Host: "/home/user/code", Container: "/work", Mode: config.MountRW}},
		Audio:         config.Audio{Pipewire: true},
		Capabilities:  config.Capabilities{Extra: []string{"NET_RAW"}},
	}
	got, err := BuildArgs(cfg, baseOpts())
	if err != nil {
		t.Fatal(err)
	}
	assertContainsSeq(t, got, "--network", "container:vpn-container")
	assertContainsSeq(t, got, "--cap-drop", "all")    // least-privilege baseline
	assertContainsSeq(t, got, "--cap-add", "NET_RAW") // explicit grant on top
	assertContains(t, got, "-d")                      // background
	assertContains(t, got, "/dev/dri")                // gpu device
	mustNotContain(t, got, "/dev/snd")                // legacy_alsa was false
	assertContainsSeq(t, got, "-v", "/home/user/code:/work:rw")
	mustNotContain(t, got, "/etc/hyprzinc/theme") // theme.mode=none → no bundle mount
	// pipewire requested → socket mounted
	assertContainsSeq(t, got, "-v", "/run/user/1000/pipewire-0:/run/hyprzinc/pipewire-0:ro")
	if got[len(got)-1] != "hyprzinc/trusted-go-dev:latest" {
		t.Fatalf("image must be last arg, got %q", got[len(got)-1])
	}
}

// Pipewire must not depend on Wayland: with audio on but no Wayland display, the
// socket is still mounted and XDG_RUNTIME_DIR is still exported.
func TestBuildArgs_PipewireWithoutWayland(t *testing.T) {
	cfg := config.AppConfig{
		SchemaVersion: config.SchemaVersion,
		App:           config.App{Name: "mpd", Image: "img@sha256:abc"},
		Display:       config.Display{Wayland: config.WaylandSecurityContext},
		Network:       config.Network{Mode: config.NetworkNone},
		Theme:         config.Theme{Mode: config.ThemeNone},
		Audio:         config.Audio{Pipewire: true},
	}
	opt := baseOpts()
	opt.WaylandDisplay = "" // headless: no Wayland socket wired
	got, err := BuildArgs(cfg, opt)
	if err != nil {
		t.Fatal(err)
	}
	assertContainsSeq(t, got, "-v", "/run/user/1000/pipewire-0:/run/hyprzinc/pipewire-0:ro")
	assertContainsSeq(t, got, "-e", "XDG_RUNTIME_DIR=/run/hyprzinc")
}

// With nothing mounted under the runtime dir, XDG_RUNTIME_DIR must NOT be set —
// otherwise apps chase a directory that isn't present in the container.
func TestBuildArgs_NoRuntimeDirWithoutSockets(t *testing.T) {
	cfg := config.AppConfig{
		SchemaVersion: config.SchemaVersion,
		App:           config.App{Name: "tool", Image: "img@sha256:abc"},
		Display:       config.Display{Wayland: config.WaylandSecurityContext},
		Network:       config.Network{Mode: config.NetworkNone},
		Theme:         config.Theme{Mode: config.ThemeNone},
	}
	opt := baseOpts()
	opt.WaylandDisplay = "" // no wayland, no audio → runtime dir stays empty
	got, err := BuildArgs(cfg, opt)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, got, "XDG_RUNTIME_DIR=/run/hyprzinc")
	assertContainsSeq(t, got, "--security-opt", "no-new-privileges")
	assertContainsSeq(t, got, "--cap-drop", "all")
}

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

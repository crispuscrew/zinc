package podman

import (
	"slices"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func baseOpts() options.HostOptions {
	return options.HostOptions{RuntimeDir: "/run/user/1000", WaylandDisplay: "wayland-1", ThemeBundleDir: "/theme", HomeDir: "/root"}
}
func netNone() []string { return []string{"--network", "none"} }
func validCfg() schema.AppConfig {
	return schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer, AppNameID: "demo", ImageMeta: schema.ImageMeta{Image: "localhost/demo:local"}}
}
func appArgs(t *testing.T, cfg schema.AppConfig, opt options.HostOptions, flags []string) []string {
	t.Helper()
	args, err := (Runtime{}).AppRunArgs(cfg, opt, flags)
	if err != nil {
		t.Fatal(err)
	}
	return args
}
func assertContains(t *testing.T, args []string, want string) {
	t.Helper()
	if !slices.Contains(args, want) {
		t.Fatalf("missing %q in %v", want, args)
	}
}
func mustNotContain(t *testing.T, args []string, unwanted string) {
	t.Helper()
	if slices.Contains(args, unwanted) {
		t.Fatalf("unexpected %q in %v", unwanted, args)
	}
}
func assertContainsSeq(t *testing.T, args []string, first, second string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return
		}
	}
	t.Fatalf("missing adjacent %q %q in %v", first, second, args)
}

func TestRunBaseline(t *testing.T) {
	cfg := validCfg()
	cfg.DisplayMeta.DisableGpuAccess = true
	args := appArgs(t, cfg, options.HostOptions{}, netNone())
	want := []string{"run", "--rm", "--pull", "never", "--name", "demo", "--security-opt", "no-new-privileges", "--cap-drop", "all", "--network", "none", cfg.ImageMeta.Image}
	if !slices.Equal(args, want) {
		t.Fatalf("got %v want %v", args, want)
	}
}

func TestCanonicalLifecycle(t *testing.T) {
	for _, mode := range []string{"foreground", "background", "terminal", "attached"} {
		for _, restart := range []bool{false, true} {
			t.Run(mode+map[bool]string{true: "/restart", false: "/normal"}[restart], func(t *testing.T) {
				cfg := validCfg()
				cfg.StopConditions.Autorestart = restart
				cfg.StopConditions.Background = mode == "background"
				cfg.StartConditions.Terminal = mode == "terminal" || mode == "attached"
				cfg.StartConditions.Attached = mode == "attached"
				args := appArgs(t, cfg, options.HostOptions{}, netNone())
				if restart {
					assertContainsSeq(t, args, "--restart", "on-failure")
					mustNotContain(t, args, "--rm")
				}
				if mode == "attached" {
					assertContains(t, args, "--init")
					assertContainsSeq(t, args, "--entrypoint", "[]")
					mustNotContain(t, args, "-it")
					if !slices.Equal(args[len(args)-2:], HolderCmd()) {
						t.Fatal(args)
					}
				} else if mode == "terminal" {
					assertContains(t, args, "-it")
				}
			})
		}
	}
	cfg := validCfg()
	cfg.StopConditions.KeepAlive = true
	cfg.StartConditions.ReadOnlyRootfs = true
	args := appArgs(t, cfg, options.HostOptions{}, nil)
	mustNotContain(t, args, "--rm")
	assertContains(t, args, "--read-only")
}

func TestFingerprintMinimizationPreservesIsolation(t *testing.T) {
	cfg := validCfg()
	cfg.MinimizeFingerprint = true
	for _, flags := range [][]string{netNone(), {"--pod", "demo-pod"}} {
		args := appArgs(t, cfg, options.HostOptions{}, flags)
		assertContains(t, args, "--unsetenv=container")
		assertContainsSeq(t, args, "--cap-drop", "all")
		assertContainsSeq(t, args, "--security-opt", "no-new-privileges")
		mustNotContain(t, args, "seccomp=unconfined")
		mustNotContain(t, args, "--privileged")
		if slices.Contains(flags, "--pod") {
			mustNotContain(t, args, "--hostname")
		} else {
			assertContainsSeq(t, args, "--hostname", "localhost")
		}
	}
}

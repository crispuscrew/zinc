package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	vmfiles "github.com/crispuscrew/zinc/common/adapters/vmoptions"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/fs"
	"github.com/crispuscrew/zinc/virtualization/runner/app"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
)

func cliFixture(t *testing.T) (app.Service, string) {
	t.Helper()
	root := t.TempDir()
	apps := filepath.Join(root, "zinc", "apps")
	if err := os.MkdirAll(apps, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "SchemaVersion: 4\nType: ZincVirtualization\nAppNameID: guest\nImageMeta:\n  Image: /images/base.qcow2\nResourcesMeta:\n  MaxRamMiB: 512\n  MaxCPUCores: 1\nStartConditions:\n  LoaderBIOS: true\n"
	if err := os.WriteFile(filepath.Join(apps, "guest.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := vmoptions.Default("guest", "/images/base.qcow2")
	runtime.BaseDigest = "sha256:" + strings.Repeat("a", 64)
	runtime.InstallMedia = []string{"/iso/stored.iso"}
	runtime.ForwardPorts = []vmoptions.PortForward{{HostPort: 2222, GuestPort: 22}}
	if err := vmfiles.Save(root, runtime); err != nil {
		t.Fatal(err)
	}
	return app.New(&fs.Store{Root: apps}, paths.Paths{}), root
}

func TestOverridesAreTransientAndListsReplace(t *testing.T) {
	svc, root := cliFixture(t)
	cfg, runtime, dry, err := launchConfig(svc, []string{"guest", "--dry-run", "--media", "/iso/new.iso", "--clear-forwards", "--disk", "0", "--runner-arg=-no-reboot"})
	if err != nil {
		t.Fatal(err)
	}
	if !dry || len(runtime.InstallMedia) != 1 || runtime.InstallMedia[0] != "/iso/new.iso" || len(runtime.ForwardPorts) != 0 {
		t.Fatal(runtime)
	}
	if len(cfg.RunnerFlags) != 1 || cfg.RunnerFlags[0] != "-no-reboot" {
		t.Fatal(cfg.RunnerFlags)
	}
	stored, err := vmfiles.Load(root, "guest")
	if err != nil || stored.InstallMedia[0] != "/iso/stored.iso" || len(stored.ForwardPorts) != 1 {
		t.Fatal("run flags were persisted")
	}
}

func TestUnknownAndSurplusArgumentsRejected(t *testing.T) {
	svc, _ := cliFixture(t)
	for _, args := range [][]string{{"guest", "--typo"}, {"guest", "other"}, {"--dry-run", "guest"}} {
		if _, _, _, err := launchConfig(svc, args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestFileAppNeedsExplicitRuntimeAuthorization(t *testing.T) {
	svc, root := cliFixture(t)
	path := filepath.Join(root, "zinc", "apps", "guest.yaml")
	if _, _, _, err := launchConfig(svc, []string{path}); err == nil {
		t.Fatal("borrowed sidecar by app name")
	}
	if _, _, _, err := launchConfig(svc, []string{path, "--runtime-options", vmfiles.File(root, "guest")}); err != nil {
		t.Fatal(err)
	}
}

func TestForwardAndResolutionParsing(t *testing.T) {
	for _, spec := range []string{"2222:22", "127.0.0.1:2222:22/TCP@primary", "[::1]:5353:53/UDP@dns"} {
		forward, err := parseForward(spec)
		if err != nil {
			t.Fatal(err)
		}
		if forward.HostPort < 1024 {
			t.Fatal(forward)
		}
	}
	for _, spec := range []string{"bad", "a:22", "2222:b"} {
		if _, err := parseForward(spec); err == nil {
			t.Fatal(spec)
		}
	}
	for _, spec := range []string{"-1x1080", "1920x0", "641x480", "1920x1080x2"} {
		if _, _, err := parseResolution(spec); err == nil {
			t.Fatal(spec)
		}
	}
}

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
)

func TestNeedsProvisioningDisc(t *testing.T) {
	for _, test := range []struct {
		cloud   bool
		devices vmoptions.Devices
		want    bool
	}{
		{true, vmoptions.DevicesVirtio, true}, {false, vmoptions.DevicesVirtio, false},
		{true, vmoptions.DevicesCompatible, true}, {false, vmoptions.DevicesCompatible, true},
	} {
		cfg := schema.AppConfig{ImageMeta: schema.ImageMeta{CloudInit: test.cloud}}
		if got := needsProvisioningDisc(cfg, vmoptions.Config{Devices: test.devices}); got != test.want {
			t.Fatalf("%+v: %v", test, got)
		}
	}
}

func serviceFixture(t *testing.T) (Service, schema.AppConfig) {
	t.Helper()
	root := t.TempDir()
	svc := New(nil, paths.Paths{StateDir: filepath.Join(root, "data"), ImageDir: filepath.Join(root, "images"), RunDir: filepath.Join(root, "run")})
	cfg := schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincVirtualization, AppNameID: "guest",
		ImageMeta: schema.ImageMeta{Image: "/images/base.qcow2"}, ResourcesMeta: schema.ResourcesMeta{MaxRamMiB: 512, MaxCPUCores: 1},
		StartConditions: schema.StartConditions{LoaderBIOS: true}, DisplayMeta: schema.DisplayMeta{DisableGpuAccess: true}}
	svc.Options = vmoptions.Default(cfg.AppNameID, cfg.ImageMeta.Image)
	svc.Options.Display = vmoptions.DisplayNone
	svc.Options.BaseDigest = "sha256:" + strings.Repeat("a", 64)
	return svc, cfg
}

func TestManifestRefusesChangedBaseAndHardware(t *testing.T) {
	svc, cfg := serviceFixture(t)
	if err := svc.Paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := svc.saveManifest(cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.checkManifest(cfg); err != nil {
		t.Fatal(err)
	}
	svc.Options.BaseDigest = "sha256:" + strings.Repeat("b", 64)
	if svc.checkManifest(cfg) == nil {
		t.Fatal("repin under existing disk accepted")
	}
	svc.Options.BaseDigest = "sha256:" + strings.Repeat("a", 64)
	cfg.StartConditions.LoaderBIOS = false
	if svc.checkManifest(cfg) == nil {
		t.Fatal("firmware changed silently")
	}
}

func TestLifecycleLockExcludesConcurrentOperations(t *testing.T) {
	svc, _ := serviceFixture(t)
	lock, err := svc.lock("guest")
	if err != nil {
		t.Fatal(err)
	}
	if other, err := svc.lock("guest"); err == nil {
		other.Close()
		t.Fatal("second lock acquired")
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := svc.lock("guest")
	if err != nil {
		t.Fatal(err)
	}
	again.Close()
	if _, err := svc.lock("../escape"); err == nil {
		t.Fatal("unsafe guest name accepted")
	}
}

func TestPlanDoesNotCreateState(t *testing.T) {
	svc, cfg := serviceFixture(t)
	// Keep Unix socket paths short independently of the test's descriptive name.
	svc.Paths.RunDir = "/run/zinc-test"
	if _, _, err := svc.Plan(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(svc.Paths.StateDir); !os.IsNotExist(err) {
		t.Fatal("planning wrote persistent state")
	}
}

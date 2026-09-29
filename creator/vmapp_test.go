package main

import (
	"os"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/store"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestNewVMWritesBothDocuments(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)
	if err := run([]string{"new", "guest", "--vm", "--image", "/images/base.qcow2", "--base-digest", testDigest,
		"--memory", "8192", "--vcpus", "4", "--disk", "40", "--display", "Compatible", "--devices", "Compatible",
		"--firmware", "UEFI", "--secure-boot", "--tpm", "--ci-user", "guest", "--ci-ssh-key", "/keys/id.pub",
		"--resolution", "1920x1080", "--media", "/iso/tools.iso, /iso/drivers.iso,", "--mac", "random"}); err != nil {
		t.Fatal(err)
	}
	sto, err := store.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := sto.Load("guest")
	if err != nil {
		t.Fatal(err)
	}
	options, err := sto.LoadVM(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Type != schema.ZincVirtualization || cfg.ResourcesMeta.MaxRamMiB != 8192 || cfg.ResourcesMeta.MaxCPUCores != 4 {
		t.Fatalf("shared settings lost: %+v", cfg)
	}
	if cfg.StartConditions.LoaderBIOS || !cfg.StartConditions.SecureBoot || !cfg.StartConditions.TPM || !cfg.ImageMeta.CloudInit || cfg.ImageMeta.PublicSSHKeyPath != "/keys/id.pub" {
		t.Fatalf("boot/provisioning fields lost: %+v", cfg)
	}
	if options.BaseDigest != testDigest || options.DiskSizeGiB != 40 || options.Display != vmoptions.DisplayCompatible || len(options.InstallMedia) != 2 {
		t.Fatalf("backend options lost: %+v", options)
	}
	if len(cfg.NetworkMeta.Interfaces) != 1 || len(cfg.NetworkMeta.RulesByPriority) != 0 {
		t.Fatal("--mac must create a NIC without granting network traffic")
	}
	for _, path := range []string{sto.Path("guest"), sto.VMPath("guest")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private definition %s: %v", path, err)
		}
	}
	if err := run([]string{"validate", "guest"}); err != nil {
		t.Fatal(err)
	}
}

func TestNewVMRequiresPinWithoutPartialYAML(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)
	err := run([]string{"new", "guest", "--vm", "--image", "/images/base.qcow2"})
	if err == nil || !strings.Contains(err.Error(), "BaseDigest") {
		t.Fatalf("want pin refusal: %v", err)
	}
	sto, _ := store.Default()
	if sto.Exists("guest") {
		t.Fatal("invalid VM left an app YAML")
	}
}

func TestNewVMFlagsWithoutVMRefused(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)
	err := run([]string{"new", "app", "--image", "localhost/app:local", "--memory", "2048"})
	if err == nil || !strings.Contains(err.Error(), "--vm") {
		t.Fatalf("want --vm error: %v", err)
	}
}

func TestNewContainerDoesNotCreateVMOptions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)
	if err := run([]string{"new", "app", "--image", "localhost/app:local"}); err != nil {
		t.Fatal(err)
	}
	sto, _ := store.Default()
	if _, err := os.Stat(sto.VMPath("app")); !os.IsNotExist(err) {
		t.Fatalf("unexpected VM options: %v", err)
	}
}

func TestRemovedTunnelIsExplicitAndWritesNothing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)
	err := run([]string{"new", "vpn", "--image", "localhost/vpn:local", "--tunnel", "/secrets/wg.conf"})
	if err == nil || !strings.Contains(err.Error(), "not representable") {
		t.Fatalf("want explicit loss error: %v", err)
	}
	sto, _ := store.Default()
	if sto.Exists("vpn") {
		t.Fatal("unsupported tunnel authored a misleading app")
	}
}

func TestDelegateUnknownAppFallsThrough(t *testing.T) {
	t.Setenv("PATH", "")
	sto := &store.Store{Root: t.TempDir()}
	err := delegate(backend.New(sto), "stop", []string{"not-an-app"})
	if err == nil || !strings.Contains(err.Error(), "zcr") {
		t.Fatalf("unknown app fallback: %v", err)
	}
}

func TestDelegateVMUnsupportedCommands(t *testing.T) {
	for command, expected := range map[string]string{"build": "no image to build", "logs": "no container log", "restart": "zc stop"} {
		if err := delegateVM(command, "guest", []string{"guest"}); err == nil || !strings.Contains(err.Error(), expected) {
			t.Errorf("%s: %v", command, err)
		}
	}
}

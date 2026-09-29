package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func TestInstallerValidatesBeforeDiskCreation(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "installer.iso")
	if err := os.WriteFile(media, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"--size", "-1"}, {"--memory", "0"}, {"--vcpus", "0"}, {"--firmware", "uefi"}, {"--devices", "Unknown"}, {"--resolution", "-1x1080"}} {
		path := filepath.Join(root, "absent", "target.qcow2")
		args := append([]string{"--disk", path, "--media", media}, extra...)
		if err := cmdInstall(args); err == nil {
			t.Fatalf("accepted %v", extra)
		}
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Fatal("invalid install wrote directories")
		}
	}
}

func TestInstallerExistingTargetNeedsExplicitResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installDisk(path, 64, false); err == nil || !strings.Contains(err.Error(), "--resume") {
		t.Fatalf("got %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "existing" {
		t.Fatal("existing disk was modified")
	}
}

func TestInstallerDoesNotUseRunnerFlags(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "media.iso")
	if err := os.WriteFile(media, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := schema.AppConfig{ResourcesMeta: schema.ResourcesMeta{MaxRamMiB: 512, MaxCPUCores: 1}, StartConditions: schema.StartConditions{LoaderBIOS: true}}
	runtime := vmoptions.Config{Image: filepath.Join(root, "disk.qcow2"), DiskSizeGiB: 1, Devices: vmoptions.DevicesCompatible, InstallMedia: []string{media}}
	if err := validateInstall(cfg, runtime); err != nil {
		t.Fatal(err)
	}
	cfg.CreatorFlags = []string{"argument\x00bad"}
	if validateInstall(cfg, runtime) == nil {
		t.Fatal("invalid raw creator argument accepted")
	}
}

package schema

import (
	"strings"
	"testing"
)

func TestMigrateVMResources(t *testing.T) {
	config, _ := migratedConfig(t, "VirtualizationMeta: {MemoryMiB: 2048, VCPUs: 2}\n")
	if config.ResourcesMeta.MaxRamMiB != 2048 || config.ResourcesMeta.MaxCPUCores != 2 ||
		config.StartConditions.LoaderBIOS || config.ImageMeta.CloudInit {
		t.Fatalf("versionless fragment contributed incorrect overrides: %+v", config)
	}
	for _, input := range []string{
		"VirtualizationMeta: {MemoryMiB: 0, VCPUs: 0}\n",
		"SchemaVersion: 3\nVirtualizationMeta: {MemoryMiB: null, VCPUs: null}\n",
		"SchemaVersion: 3\nVirtualizationMeta: {MemoryMiB: 0o0, VCPUs: 0x0}\n",
	} {
		_, output := migratedConfig(t, input)
		if strings.Contains(output, "ResourcesMeta:") || strings.Contains(output, "VirtualizationMeta:") {
			t.Fatalf("zero legacy resource residue created overrides: %s", output)
		}
	}
	config, _ = migratedConfig(t, `SchemaVersion: 3
Type: ZincContainer
ResourcesMeta: {MaxRamMiB: 512, MaxCPUCores: 0.5}
VirtualizationMeta: {MemoryMiB: 0, VCPUs: 0}
`)
	if config.ResourcesMeta.MaxRamMiB != 512 || config.ResourcesMeta.MaxCPUCores != 0.5 {
		t.Fatal("container resources changed")
	}
}

func TestMigrateVMDefaultsAndFields(t *testing.T) {
	for _, version := range []string{"3", "4"} {
		for _, testCase := range []struct {
			legacy      string
			bios, cloud bool
		}{
			{"{}", true, true},
			{"{Firmware: BIOS, CloudInit: {Disabled: false}}", true, true},
			{"{Firmware: UEFI, CloudInit: {Disabled: true}}", false, false},
			{"{Firmware: null, CloudInit: null}", true, true},
		} {
			config, _ := migratedConfig(t, "SchemaVersion: "+version+"\nType: ZincVirtualization\nInherits: ''\nVirtualizationMeta: "+testCase.legacy+"\n")
			if config.StartConditions.LoaderBIOS != testCase.bios || config.ImageMeta.CloudInit != testCase.cloud {
				t.Fatalf("old defaults inverted incorrectly for %s: %+v", testCase.legacy, config)
			}
		}
	}
	config, _ := migratedConfig(t, `SchemaVersion: 4
Type: ZincVirtualization
VirtualizationMeta:
  DisplayWidth: 1920
  DisplayHeight: 1080
  Vulkan: true
  Firmware: UEFI
  SecureBoot: true
  TPM: true
  CloudInit: {SSHKeyPath: /keys/public.pub}
`)
	if config.DisplayMeta.DisplayWidth != 1920 || config.DisplayMeta.DisplayHeight != 1080 ||
		!config.DisplayMeta.Vulkan || !config.StartConditions.SecureBoot || !config.StartConditions.TPM ||
		config.ImageMeta.PublicSSHKeyPath != "/keys/public.pub" || !config.ImageMeta.CloudInit {
		t.Fatalf("representable VM settings lost: %+v", config)
	}
}

func TestMigrateNullResourceParent(t *testing.T) {
	config, output := migratedConfig(t, `SchemaVersion: 3
Type: ZincVirtualization
ResourcesMeta: &empty null
DisplayMeta: *empty
VirtualizationMeta: {MemoryMiB: 1, VCPUs: 1}
`)
	if config.ResourcesMeta.MaxRamMiB != 1 || !strings.Contains(output, "DisplayMeta: null") {
		t.Fatalf("null alias changed: %s", output)
	}
}

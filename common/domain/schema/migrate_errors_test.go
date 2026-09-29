package schema

import (
	"strings"
	"testing"
)

func TestMigrateFailsClosed(t *testing.T) {
	for _, testCase := range []struct{ input, field string }{
		{"Icon: old\nLauncherMeta: {Icon: new}\n", "Icon"},
		{"Env: {}\nStartConditions: {EntrypointEnv: {}}\n", "EntrypointEnv"},
		{"ReadOnlyRootfs: false\nStartConditions: {ReadOnlyRootfs: false}\n", "ReadOnlyRootfs"},
		{"StartConditions: {Autorestart: false}\nStopConditions: {Autorestart: false}\n", "Autorestart"},
		{"StartConditions: {Multiterminal: false, Attached: false}\n", "Attached"},
		{"ResourcesMeta: {MaxSwapMiB: 1}\n", "MaxSwapMiB"},
		{"ResourcesMeta: {MaxSwapMiB: '0'}\n", "MaxSwapMiB"},
		{"StartConditions: {ReadyCheck: [ready]}\n", "ReadyCheck"},
		{"StartConditions: {ReadyTimeoutSec: 1}\n", "ReadyTimeoutSec"},
		{"Capabilities: [NET_RAW]\n", "Capabilities"},
		{"NetworkMeta: {Tunnel: {WireGuardConf: /etc/wg.conf}}\n", "WireGuardConf"},
		{"NetworkMeta: {Tunnel: {Unknown: false}}\n", "Tunnel"},
		{"Type: ZincContainer\nVirtualizationMeta: {MemoryMiB: 1}\n", "MemoryMiB"},
		{"VirtualizationMeta: {BaseDigest: 'sha256:abc'}\n", "BaseDigest"},
		{"VirtualizationMeta: {DiskSizeGiB: 10}\n", "DiskSizeGiB"},
		{"VirtualizationMeta: {Display: None}\n", "Display"},
		{"VirtualizationMeta: {Devices: Compatible}\n", "Devices"},
		{"VirtualizationMeta: {InstallMedia: [/installer.iso]}\n", "InstallMedia"},
		{"VirtualizationMeta: {ForwardPorts: [{HostPort: 2222, GuestPort: 22}]}\n", "ForwardPorts"},
		{"VirtualizationMeta: {MacAddress: '02:00:00:00:00:01'}\n", "MacAddress"},
		{"VirtualizationMeta: {CloudInit: {UserName: guest}}\n", "UserName"},
		{"VirtualizationMeta: {Firmware: wrong}\n", "Firmware"},
		{"VirtualizationMeta: {Firmware: BIOS}\nStartConditions: {LoaderBIOS: true}\n", "LoaderBIOS"},
		{"VirtualizationMeta: {CloudInit: {Disabled: false}}\nImageMeta: {CloudInit: true}\n", "CloudInit"},
		{"VirtualizationMeta: {MemoryMiB: 8192}\nResourcesMeta: {MaxRamMiB: 4096}\n", "MaxRamMiB"},
		{"AudioMeta: {Microphone: yes}\n", "Microphone"},
		{"AudioMeta: {Microphone: [speaker]}\n", "Microphone"},
		{"SchemaVersion: 3\nIcon: first\nIcon: second\n", "Icon"},
		{"SchemaVersion: 3\n---\nIcon: second\n", "one YAML document"},
		{"Env: &recursive {LOOP: *recursive}\n", "anchor"},
	} {
		t.Run(testCase.field, func(t *testing.T) {
			_, err := Migrate([]byte(testCase.input))
			if err == nil || !strings.Contains(err.Error(), testCase.field) {
				t.Fatalf("want actionable %s error, got %v", testCase.field, err)
			}
		})
	}
}

func TestMigrateRemovesOnlyZeroResidue(t *testing.T) {
	_, output := migratedConfig(t, `SchemaVersion: 4
Type: ZincContainer
Capabilities: []
ResourcesMeta: {MaxSwapMiB: null, MaxRamMiB: 512}
StartConditions: {ReadyCheck: [], ReadyTimeoutSec: 0}
NetworkMeta: {Tunnel: {WireGuardConf: ''}}
VirtualizationMeta:
  BaseDigest: ''
  DiskSizeGiB: 0
  Display: ''
  Firmware: ''
  Devices: ''
  InstallMedia: []
  ForwardPorts: []
  CloudInit: {Disabled: false, UserName: '', SSHKeyPath: ''}
`)
	for _, legacy := range []string{"Capabilities", "MaxSwapMiB", "ReadyCheck", "ReadyTimeoutSec", "Tunnel", "VirtualizationMeta"} {
		if strings.Contains(output, legacy) {
			t.Fatalf("zero residue %s survived: %s", legacy, output)
		}
	}
}

func TestMigrateMergeAliases(t *testing.T) {
	config, _ := migratedConfig(t, `SchemaVersion: 3
StartConditions:
  <<: &legacy {Multiterminal: true}
  Terminal: false
LauncherMeta: {Description: sample}
`)
	if !config.StartConditions.Attached || config.StartConditions.Terminal {
		t.Fatalf("merge alias values changed: %+v", config.StartConditions)
	}
}

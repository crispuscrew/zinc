package schema

import (
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func migratedConfig(t *testing.T, input string) (AppConfig, string) {
	t.Helper()
	output, err := Migrate([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	var config AppConfig
	decoder := yaml.NewDecoder(bytes.NewReader(output))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		t.Fatalf("migration did not produce strict current YAML: %v\n%s", err, output)
	}
	again, err := Migrate(output)
	if err != nil || !bytes.Equal(again, output) {
		t.Fatalf("migration is not idempotent: %v\n%s\n%s", err, output, again)
	}
	return config, string(output)
}

func TestMigrateRenames(t *testing.T) {
	for _, version := range []string{"", "SchemaVersion: 3\n", "SchemaVersion: 4\n"} {
		config, _ := migratedConfig(t, version+`Type: ZincContainer
Icon: app
Description: description
Group: tools
Env: {LANG: C}
ReadOnlyRootfs: true
StartConditions:
  Multiterminal: true
  MultiterminalEntrypoint: /bin/sh
  MultiterminalEnv: {TERM: xterm}
  Autorestart: true
`)
		if config.LauncherMeta != (LauncherMeta{Icon: "app", Description: "description", Group: "tools"}) ||
			config.StartConditions.EntrypointEnv["LANG"] != "C" || !config.StartConditions.ReadOnlyRootfs ||
			!config.StartConditions.Attached || config.StartConditions.AttachedEntrypoint != "/bin/sh" ||
			config.StartConditions.AttachedEnv["TERM"] != "xterm" || !config.StopConditions.Autorestart {
			t.Fatalf("renamed fields lost: %+v", config)
		}
		if version == "" && config.SchemaVersion != 0 {
			t.Fatal("versionless fragment acquired a version override")
		}
	}
}

func TestMigrateCurrentSchemaIsUntouched(t *testing.T) {
	for _, input := range []string{
		"# comment\nSchemaVersion: 4\nType: ZincVirtualization\nImageMeta: {CloudInit: false}\n",
		"SchemaVersion: 4\nAudioMeta: {Playback: {PipeWireDefault: true}}\n",
		"StartConditions: {EntrypointEnv: &env {LANG: C}, AttachedEnv: *env}\n",
		"SchemaVersion: 9\nIcon: future\n",
	} {
		output, err := Migrate([]byte(input))
		if err != nil || string(output) != input {
			t.Fatalf("document rewritten: %v\n%s", err, output)
		}
	}
}

func TestMigrateNullsAndAliases(t *testing.T) {
	config, output := migratedConfig(t, `SchemaVersion: &limit 0o3
Description: &vmtype ZincVirtualization
Type: *vmtype
ResourcesMeta:
  MaxRamMiB: 0
  MaxCPUCores: 0
  MaxSwapMiB: 0
  PIDsLimit: *limit
VirtualizationMeta:
  &memory MemoryMiB: 8192
  VCPUs: 4
Icon: *memory
Env: &empty null
DisplayMeta: *empty
AudioMeta: {Microphone: null}
`)
	if config.ResourcesMeta.MaxRamMiB != 8192 || config.ResourcesMeta.MaxCPUCores != 4 ||
		config.ResourcesMeta.PIDsLimit != 3 || config.LauncherMeta.Icon != "MemoryMiB" {
		t.Fatalf("aliased values changed: %+v", config)
	}
	if !strings.Contains(output, "DisplayMeta: null") || !strings.Contains(output, "Microphone: null") ||
		!strings.Contains(output, "EntrypointEnv: &empty null") {
		t.Fatalf("explicit nulls were lost: %s", output)
	}
}

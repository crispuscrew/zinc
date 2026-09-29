package inherit

import "testing"

func TestMergeAliasesKeepOriginalValues(t *testing.T) {
	output, err := Merge([]byte(`StartConditions:
  EntrypointEnv: &environment {LANG: C, LD_PRELOAD: /base.so}
  AttachedEnv: *environment
AudioMeta:
  Playback: &audio {PipeWireDefault: true}
  Microphone: *audio
`), []byte(`StartConditions:
  EntrypointEnv: &environment {}
AudioMeta:
  Playback: &audio {}
`))
	if err != nil {
		t.Fatal(err)
	}
	config := strictConfig(t, output)
	if len(config.StartConditions.EntrypointEnv) != 0 || config.StartConditions.AttachedEnv["LD_PRELOAD"] != "/base.so" ||
		!config.AudioMeta.Playback.IsZero() || !config.AudioMeta.Microphone.PipeWireDefault {
		t.Fatalf("overriding an anchor changed an unrelated alias: %+v", config)
	}
}

func TestResolveMergeKeysAndNulls(t *testing.T) {
	config := resolveFrom(t, "child", map[string]string{
		"base": "StartConditions: {AttachedEnv: {LD_PRELOAD: /base.so}, EntrypointEnv: {LANG: C}}\nAudioMeta: {Monitor: {PipeWireDefault: true}}\n",
		"child": `Inherits: base
StartConditions:
  <<: &overrides {AttachedEnv: {}, EntrypointEnv: null}
AudioMeta: {Monitor: null}
`,
	})
	if len(config.StartConditions.AttachedEnv) != 0 || config.StartConditions.EntrypointEnv != nil || !config.AudioMeta.Monitor.IsZero() {
		t.Fatalf("merged explicit empty/null grants were ignored: %+v", config)
	}
}

func TestResolveVersionlessVMResources(t *testing.T) {
	config := resolveFrom(t, "child", map[string]string{
		"base":  "SchemaVersion: 3\nType: ZincVirtualization\nVirtualizationMeta: {MemoryMiB: 1024, VCPUs: 1, Firmware: UEFI, CloudInit: {Disabled: true}}\n",
		"child": "Inherits: base\nVirtualizationMeta: {MemoryMiB: 2048}\n",
	})
	if config.ResourcesMeta.MaxRamMiB != 2048 || config.ResourcesMeta.MaxCPUCores != 1 ||
		config.StartConditions.LoaderBIOS || config.ImageMeta.CloudInit {
		t.Fatalf("legacy fragment overwrote absent fields/defaults: %+v", config)
	}
}

func TestResolveVMCloudKeyDoesNotEnableCloudInit(t *testing.T) {
	config := resolveFrom(t, "child", map[string]string{
		"base":  "SchemaVersion: 3\nType: ZincVirtualization\nVirtualizationMeta: {CloudInit: {Disabled: true}}\n",
		"child": "Inherits: base\nVirtualizationMeta: {CloudInit: {SSHKeyPath: /keys/public.pub}}\n",
	})
	if config.ImageMeta.CloudInit || config.ImageMeta.PublicSSHKeyPath != "/keys/public.pub" {
		t.Fatalf("an SSH key override changed inherited cloud-init: %+v", config.ImageMeta)
	}
}

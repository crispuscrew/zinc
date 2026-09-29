package inherit

import (
	"fmt"
	"testing"
)

func TestResolveEnvironmentReplacesWhole(t *testing.T) {
	for _, field := range []string{"EntrypointEnv", "AttachedEnv"} {
		base := fmt.Sprintf("StartConditions:\n  %s: {LANG: C, LD_PRELOAD: /hook.so}\n", field)
		for _, replacement := range []string{"{}", "{LANG: en_US.UTF-8}", "null"} {
			config := resolveFrom(t, "child", map[string]string{
				"base":  base,
				"child": fmt.Sprintf("Inherits: base\nStartConditions:\n  %s: %s\n", field, replacement),
			})
			environment := config.StartConditions.EntrypointEnv
			if field == "AttachedEnv" {
				environment = config.StartConditions.AttachedEnv
			}
			if _, exists := environment["LD_PRELOAD"]; exists {
				t.Fatalf("%s accumulated inherited environment: %v", field, environment)
			}
			if replacement == "{LANG: en_US.UTF-8}" && environment["LANG"] != "en_US.UTF-8" {
				t.Fatal("child environment lost")
			}
		}
	}
	config := resolveFrom(t, "child", map[string]string{
		"base":  "SchemaVersion: 3\nEnv: {LD_PRELOAD: /hook.so}\n",
		"child": "Inherits: base\nEnv: {}\n",
	})
	if len(config.StartConditions.EntrypointEnv) != 0 {
		t.Fatal("migrated Env: {} did not clear the inherited map")
	}
	config = resolveFrom(t, "child", map[string]string{
		"base":  "StartConditions: {EntrypointEnv: {LD_PRELOAD: /hook.so}}\n",
		"child": "Inherits: base\n",
	})
	if config.StartConditions.EntrypointEnv["LD_PRELOAD"] != "/hook.so" {
		t.Fatal("an omitted environment must inherit")
	}
}

func TestResolveAudioDirectionsReplaceWhole(t *testing.T) {
	for _, direction := range []string{"Playback", "Microphone", "Monitor"} {
		for _, replacement := range []string{"{}", "null", "{PipeWireDefault: false}", "{ALSADevices: [/dev/snd/controlC0]}"} {
			config := resolveFrom(t, "child", map[string]string{
				"base":  fmt.Sprintf("AudioMeta:\n  %s: {PipeWireDefault: true, PipeWireDevices: [capture], ALSADevices: [/dev/snd/pcmC0D0c]}\n", direction),
				"child": fmt.Sprintf("Inherits: base\nAudioMeta:\n  %s: %s\n", direction, replacement),
			})
			device := config.AudioMeta.Playback
			if direction == "Microphone" {
				device = config.AudioMeta.Microphone
			} else if direction == "Monitor" {
				device = config.AudioMeta.Monitor
			}
			if device.PipeWireDefault || len(device.PipeWireDevices) != 0 {
				t.Fatalf("%s accumulated inherited grants: %+v", direction, device)
			}
			if replacement == "{ALSADevices: [/dev/snd/controlC0]}" {
				if len(device.ALSADevices) != 1 || device.ALSADevices[0] != "/dev/snd/controlC0" {
					t.Fatalf("ALSA replacement failed: %+v", device)
				}
			} else if !device.IsZero() {
				t.Fatalf("explicit empty grant did not clear: %+v", device)
			}
		}
	}
}

func TestResolveLegacyAudioShapes(t *testing.T) {
	for _, forms := range [][2]string{{"default", "[/dev/snd/pcmC0D0c]"}, {"[/dev/snd/pcmC0D0c]", "none"}} {
		config := resolveFrom(t, "child", map[string]string{
			"base":  "SchemaVersion: 4\nAudioMeta: {Playback: default, Microphone: " + forms[0] + "}\n",
			"child": "Inherits: base\nAudioMeta: {Microphone: " + forms[1] + "}\n",
		})
		if !config.AudioMeta.Playback.PipeWireDefault || config.AudioMeta.Microphone.PipeWireDefault {
			t.Fatalf("omitted direction or explicit replacement changed: %+v", config.AudioMeta)
		}
		if forms[1] == "none" && !config.AudioMeta.Microphone.IsZero() {
			t.Fatal("legacy none retained inherited audio")
		}
	}
}

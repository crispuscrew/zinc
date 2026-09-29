package schema

import "testing"

func TestMigrateLegacyAudio(t *testing.T) {
	for _, version := range []string{"", "SchemaVersion: 3\n", "SchemaVersion: 4\n"} {
		config, _ := migratedConfig(t, version+`AudioMeta:
  Playback: &grant default
  Microphone: [/dev/snd/controlC0, /dev/snd/pcmC0D0c]
  Monitor: none
LauncherMeta: {Description: *grant}
`)
		if !config.AudioMeta.Playback.PipeWireDefault || len(config.AudioMeta.Microphone.ALSADevices) != 2 ||
			!config.AudioMeta.Monitor.IsZero() || config.LauncherMeta.Description != "default" {
			t.Fatalf("migration changed an audio grant or alias: %+v", config)
		}
	}
	config, _ := migratedConfig(t, "AudioMeta: {Playback: [], Microphone: null}\n")
	if !config.AudioMeta.Playback.IsZero() || !config.AudioMeta.Microphone.IsZero() {
		t.Fatal("empty legacy audio gained a grant")
	}
}

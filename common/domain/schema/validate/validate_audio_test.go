package validate

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestStructuredAudioGrantsAreAdditive(test *testing.T) {
	for _, base := range []func() schema.AppConfig{baseCfg, baseVM} {
		cfg := base()
		cfg.AudioMeta.Playback = schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"alsa_output.usb-card.stereo"}, ALSADevices: []string{"/dev/snd/controlC0", "/dev/snd/pcmC0D0p"}}
		cfg.AudioMeta.Microphone = schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"alsa_input.card"}, ALSADevices: []string{"/dev/snd/controlC0", "/dev/snd/pcmC0D0c"}}
		cfg.AudioMeta.Monitor = schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"alsa_output.usb-card.stereo"}}
		if err := Validate(cfg); err != nil {
			test.Fatal(err)
		}
	}
}

func TestAudioExactNodesAndDirection(test *testing.T) {
	for _, node := range []string{"", "/etc/shadow", "/dev/dri/renderD128", "/dev/snd", "/dev/snd/", "/dev/snd/*", "/dev/snd/../dri/renderD128", "/dev/snd//controlC0", "/dev/snd/controlC0,file=x", "/dev/snd/control C0", "/dev/snd/controlC0:/device", "/dev/snd/seq", "/dev/snd/timer", "/dev/snd/hwC0D0", "/dev/snd/not-a-node"} {
		cfg := baseCfg()
		cfg.AudioMeta.Microphone.ALSADevices = []string{node}
		requireError(test, cfg, "ALSADevices")
	}
	cfg := baseCfg()
	cfg.AudioMeta.Playback.ALSADevices = []string{"/dev/snd/pcmC0D0c"}
	requireError(test, cfg, "CAPTURE device")
	cfg.AudioMeta.Playback = schema.AudioDevice{}
	cfg.AudioMeta.Microphone.ALSADevices = []string{"/dev/snd/pcmC0D0p"}
	requireError(test, cfg, "PLAYBACK device")
	for _, selector := range []string{"", " ", "sink\nname", "sink\x00name", "sink.*", "~alsa_output.*", "sink?", "sink[12]", "{node}", " sink"} {
		cfg = baseCfg()
		cfg.AudioMeta.Playback.PipeWireDevices = []string{selector}
		requireError(test, cfg, "exact node.name")
	}
}

func TestMonitorALSARequiresResolvedLoopback(test *testing.T) {
	for _, base := range []func() schema.AppConfig{baseCfg, baseVM} {
		for _, node := range []string{"/dev/snd/controlC0", "/dev/snd/pcmC0D0c"} {
			cfg := base()
			cfg.AudioMeta.Monitor.ALSADevices = []string{node}
			requireError(test, cfg, "loopback")
		}
	}
}

func TestPrivatePlaybackDoesNotImplyHostMonitorExposure(test *testing.T) {
	for _, playback := range []schema.AudioDevice{{PipeWireDefault: true}, {PipeWireDevices: []string{"alsa_output.card"}}} {
		cfg := baseCfg()
		cfg.AudioMeta.Playback = playback
		cfg.AudioMeta.Microphone.PipeWireDefault = true
		if warnings := Warnings(cfg); len(warnings) != 0 {
			test.Fatalf("private playback endpoints isolate the host mix: %v", warnings)
		}
		cfg.AudioMeta.Monitor = playback
		if warnings := Warnings(cfg); len(warnings) != 0 {
			test.Fatalf("explicit monitor grant: %v", warnings)
		}
	}
	cfg := baseCfg()
	cfg.AudioMeta.Playback.ALSADevices = []string{"/dev/snd/controlC0", "/dev/snd/pcmC0D0p"}
	if warnings := Warnings(cfg); len(warnings) != 0 {
		test.Fatalf("exact ALSA playback nodes: %v", warnings)
	}
	cfg.AudioMeta.Playback.PipeWireDevices = []string{"alsa_output.nondefault"}
	cfg.AudioMeta.Monitor.PipeWireDefault = true
	if warnings := Warnings(cfg); len(warnings) != 0 {
		test.Fatalf("playback and monitor selections are independently brokered: %v", warnings)
	}
}

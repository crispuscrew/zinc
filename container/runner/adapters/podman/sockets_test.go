package podman

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func TestWaylandSocketIdentity(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		cfg := validCfg()
		cfg.DisplayMeta.DisableSecurityContext = disabled
		opt := baseOpts()
		opt.WaylandSocket = "/restricted/wayland-1"
		args := appArgs(t, cfg, opt, netNone())
		if disabled {
			assertContainsSeq(t, args, "-v", "/run/user/1000/wayland-1:/run/zinc/wayland-1:ro")
			assertContainsSeq(t, args, "--label", "zinc.wayland=passthrough")
		} else {
			assertContainsSeq(t, args, "-v", "/restricted/wayland-1:/run/zinc/wayland-1:ro")
			assertContainsSeq(t, args, "--label", "zinc.wayland=security-context")
		}
		assertContainsSeq(t, args, "-e", "WAYLAND_DISPLAY=wayland-1")
	}
}

func TestAudioRequiresRestrictedSocket(t *testing.T) {
	for _, device := range []schema.AudioDevice{{PipeWireDefault: true}, {PipeWireDevices: []string{"exact.node"}}} {
		for _, direction := range []string{"playback", "microphone", "monitor"} {
			cfg := validCfg()
			switch direction {
			case "playback":
				cfg.AudioMeta.Playback = device
			case "microphone":
				cfg.AudioMeta.Microphone = device
			case "monitor":
				cfg.AudioMeta.Monitor = device
			}
			if _, err := (Runtime{}).AppRunArgs(cfg, baseOpts(), nil); err == nil {
				t.Fatal("raw PipeWire fallback accepted")
			}
			opt := options.HostOptions{PipeWireSocket: "/restricted/pipewire-0"}
			args := appArgs(t, cfg, opt, nil)
			assertContainsSeq(t, args, "-v", "/restricted/pipewire-0:/run/zinc/pipewire-0:ro")
			assertContainsSeq(t, args, "-e", "XDG_RUNTIME_DIR=/run/zinc")
		}
	}
}

func TestALSAExactNodesDeduplicated(t *testing.T) {
	cfg := validCfg()
	cfg.AudioMeta.Playback.ALSADevices = []string{"/dev/snd/controlC0", "/dev/snd/pcmC0D0p"}
	cfg.AudioMeta.Microphone.ALSADevices = []string{"/dev/snd/controlC0", "/dev/snd/pcmC0D0c"}
	args := appArgs(t, cfg, options.HostOptions{}, nil)
	assertContainsSeq(t, args, "--group-add", "keep-groups")
	for _, device := range []string{"/dev/snd/controlC0", "/dev/snd/pcmC0D0p", "/dev/snd/pcmC0D0c"} {
		count := 0
		for _, arg := range args {
			if arg == device {
				count++
			}
		}
		if count != 1 {
			t.Fatal(args)
		}
		assertContainsSeq(t, args, "--device", device)
	}
	mustNotContain(t, args, "/dev/snd")
	mustNotContain(t, args, "XDG_RUNTIME_DIR=/run/zinc")
}

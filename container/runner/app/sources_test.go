package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func TestALSAHostSourcesMustBeCharacterDevices(t *testing.T) {
	cfg := depApp("audio")
	file := filepath.Join(t.TempDir(), "pcmC0D0p")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.AudioMeta.Playback.ALSADevices = []string{file}
	if err := checkLaunchSources(cfg, options.HostOptions{}); err == nil || !strings.Contains(err.Error(), "character device") {
		t.Fatalf("regular file accepted: %v", err)
	}
	cfg.AudioMeta.Playback.ALSADevices = []string{file + "-missing"}
	if err := checkLaunchSources(cfg, options.HostOptions{}); err == nil || !strings.Contains(err.Error(), "not on this host") {
		t.Fatal(err)
	}
	cfg.AudioMeta.Playback.ALSADevices = []string{"/dev/null"}
	if err := checkLaunchSources(cfg, options.HostOptions{}); err != nil {
		t.Fatal(err)
	}
	// Host existence does not replace canonical path and direction validation.
	for _, audio := range []schema.AudioMeta{
		{Playback: schema.AudioDevice{ALSADevices: []string{"/dev/snd/pcmC0D0c"}}},
		{Microphone: schema.AudioDevice{ALSADevices: []string{"/dev/snd/pcmC0D0p"}}},
		{Playback: schema.AudioDevice{ALSADevices: []string{"/dev/null"}}},
	} {
		cfg.AudioMeta = audio
		if err := validate.Validate(cfg); err == nil {
			t.Fatalf("invalid ALSA grant accepted: %+v", audio)
		}
	}
}

func TestConfigSourcesRemainInsideBundle(t *testing.T) {
	bundle := t.TempDir()
	cfg := depApp("demo")
	cfg.Configs = []schema.ConfigFile{{BundlePath: "app.json", InnerMount: "/etc/app.json"}}
	opt := options.HostOptions{BundleDir: bundle}
	if err := checkLaunchSources(cfg, opt); err == nil {
		t.Fatal("missing config accepted")
	}
	if err := os.WriteFile(filepath.Join(bundle, "app.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkLaunchSources(cfg, opt); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hosts", filepath.Join(bundle, "outside.json")); err != nil {
		t.Fatal(err)
	}
	cfg.Configs[0].BundlePath = "outside.json"
	if err := checkLaunchSources(cfg, opt); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatal(err)
	}
}

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func checkLaunchSources(cfg schema.AppConfig, opt options.HostOptions) error {
	for _, config := range cfg.Configs {
		source := filepath.Join(opt.BundleDir, config.BundlePath)
		info, err := os.Stat(source)
		if err != nil {
			return fmt.Errorf("%s: Configs %q: %w; the app's bundle is %s", cfg.AppNameID, config.BundlePath, err, opt.BundleDir)
		}
		if info.IsDir() {
			return fmt.Errorf("%s: Configs %q resolves to a directory (%s)", cfg.AppNameID, config.BundlePath, source)
		}
		real, err := filepath.EvalSymlinks(source)
		if err != nil {
			return err
		}
		bundle, err := filepath.EvalSymlinks(opt.BundleDir)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(real, bundle+string(filepath.Separator)) {
			return fmt.Errorf("%s: Configs %q leads outside the app's bundle (to %s)", cfg.AppNameID, config.BundlePath, real)
		}
	}
	for _, direction := range []struct {
		name  string
		nodes []string
	}{
		{"Playback", cfg.AudioMeta.Playback.ALSADevices}, {"Microphone", cfg.AudioMeta.Microphone.ALSADevices},
		{"Monitor", cfg.AudioMeta.Monitor.ALSADevices},
	} {
		for _, node := range direction.nodes {
			info, err := os.Stat(node)
			if err != nil {
				return fmt.Errorf("%s: AudioMeta.%s names %s, which is not on this host: %w", cfg.AppNameID, direction.name, node, err)
			}
			if info.Mode()&os.ModeCharDevice == 0 {
				return fmt.Errorf("%s: AudioMeta.%s %s must be a host character device", cfg.AppNameID, direction.name, node)
			}
		}
	}
	return nil
}

func wantsPipeWire(audio schema.AudioMeta) bool {
	for _, device := range []schema.AudioDevice{audio.Playback, audio.Microphone, audio.Monitor} {
		if device.PipeWireDefault || len(device.PipeWireDevices) > 0 {
			return true
		}
	}
	return false
}

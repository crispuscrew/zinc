package disk

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func WriteSeed(path string, cfg schema.AppConfig, devices vmoptions.Devices) error {
	files, err := seedFiles(cfg, devices)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no provisioning data requested")
	}
	stage, err := os.MkdirTemp(filepath.Dir(path), ".seed-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	names := slices.Sorted(maps.Keys(files))
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(stage, name), []byte(files[name]), 0o600); err != nil {
			return err
		}
	}
	outputPath := filepath.Join(stage, "seed.iso")
	args := []string{"-as", "mkisofs", "-output", outputPath, "-volid", "cidata", "-joliet", "-rock"}
	for _, name := range names {
		args = append(args, filepath.Join(stage, name))
	}
	if output, err := exec.Command("xorriso", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("build provisioning image: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Chmod(outputPath, 0o600); err != nil {
		return err
	}
	return os.Rename(outputPath, path)
}

func seedFiles(cfg schema.AppConfig, devices vmoptions.Devices) (map[string]string, error) {
	files := map[string]string{}
	if cfg.ImageMeta.CloudInit {
		user, err := userData(cfg)
		if err != nil {
			return nil, err
		}
		metadata, err := metaData(cfg)
		if err != nil {
			return nil, err
		}
		files["user-data"], files["meta-data"] = user, metadata
	}
	// A hardware profile is not an OS detector. The optional script is inert
	// unless an operator chooses to run it inside a compatible Windows guest.
	if devices == vmoptions.DevicesCompatible {
		files["zinc-setup.cmd"] = windowsSetup()
	}
	return files, nil
}

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

type manifest struct {
	Image, Digest         string
	Devices               vmoptions.Devices
	BIOS, SecureBoot, TPM bool
}

func (svc Service) manifestPath(name string) string {
	return filepath.Join(svc.Paths.StateDir, name+".machine.json")
}

func (svc Service) manifest(cfg schema.AppConfig) manifest {
	devices := svc.Options.Devices
	if devices == "" {
		devices = vmoptions.DevicesVirtio
	}
	return manifest{cfg.ImageMeta.Image, svc.Options.BaseDigest, devices,
		cfg.StartConditions.LoaderBIOS, cfg.StartConditions.SecureBoot, cfg.StartConditions.TPM}
}

func (svc Service) checkManifest(cfg schema.AppConfig) error {
	data, err := os.ReadFile(svc.manifestPath(cfg.AppNameID))
	if os.IsNotExist(err) {
		return nil // existing pre-manifest overlays still undergo backing-file checks
	}
	if err != nil {
		return err
	}
	var existing manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&existing); err != nil {
		return fmt.Errorf("invalid VM state manifest: %w", err)
	}
	if existing != svc.manifest(cfg) {
		return fmt.Errorf("VM base pin, device profile or firmware differs from existing state; explicit offline migration is required")
	}
	return nil
}

func (svc Service) saveManifest(cfg schema.AppConfig) error {
	if err := svc.checkManifest(cfg); err != nil {
		return err
	}
	path := svc.manifestPath(cfg.AppNameID)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := json.MarshalIndent(svc.manifest(cfg), "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(svc.Paths.StateDir, ".machine-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Link(file.Name(), path)
}

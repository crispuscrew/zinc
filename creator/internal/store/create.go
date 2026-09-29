package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	vmfiles "github.com/crispuscrew/zinc/common/adapters/vmoptions"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

// Create validates both documents before publishing either. Hard links provide
// no-replace publication, including when another creator races this one.
func (sto *Store) Create(cfg schema.AppConfig, options *vmoptions.Config) error {
	if err := checkDefinition(cfg, options); err != nil {
		return err
	}
	if err := os.MkdirAll(sto.Root, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(sto.Root, ".create-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	data, err := Marshal(cfg)
	if err != nil {
		return err
	}
	appFile := filepath.Join(stage, "app.yaml")
	if err := os.WriteFile(appFile, data, 0o600); err != nil {
		return err
	}
	var optionFile, destination string
	if options != nil {
		if err := vmfiles.Save(stage, *options); err != nil {
			return err
		}
		optionFile, destination = vmfiles.File(stage, cfg.AppNameID), sto.VMPath(cfg.AppNameID)
		if err := publish(optionFile, destination); err != nil {
			return err
		}
	}
	if err := publish(appFile, sto.Path(cfg.AppNameID)); err != nil {
		if options != nil {
			return errors.Join(err, removePublished(optionFile, destination))
		}
		return err
	}
	return nil
}

func checkDefinition(cfg schema.AppConfig, options *vmoptions.Config) error {
	if strings.TrimSpace(cfg.Inherits) != "" {
		return fmt.Errorf("inheriting apps must be edited as sparse YAML files")
	}
	if err := validate.Validate(cfg); err != nil {
		return err
	}
	if cfg.Type != schema.ZincVirtualization {
		if options != nil {
			return fmt.Errorf("VM runtime options cannot be written for a container app")
		}
		return nil
	}
	if options == nil {
		return fmt.Errorf("VM app requires its runtime options (including BaseDigest)")
	}
	if options.AppNameID != cfg.AppNameID || options.Image != cfg.ImageMeta.Image {
		return fmt.Errorf("VM runtime options must match AppNameID and ImageMeta.Image")
	}
	return vmoptions.Validate(*options)
}

func publish(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	if err := os.Link(source, destination); err != nil {
		return fmt.Errorf("create %s without overwriting: %w", destination, err)
	}
	return nil
}

// Never remove a file another writer replaced after our publication.
func removePublished(source, destination string) error {
	created, err := os.Stat(source)
	if err != nil {
		return err
	}
	current, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(created, current) {
		return fmt.Errorf("rollback: %s changed concurrently; retained it", destination)
	}
	return os.Remove(destination)
}

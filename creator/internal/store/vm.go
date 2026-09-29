package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	vmfiles "github.com/crispuscrew/zinc/common/adapters/vmoptions"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func (sto *Store) ConfigRoot() string {
	if filepath.Base(sto.Root) == "apps" && filepath.Base(filepath.Dir(sto.Root)) == "zinc" {
		return filepath.Dir(filepath.Dir(sto.Root))
	}
	return sto.Root // custom/test stores keep all their state under their own root
}

func (sto *Store) VMPath(name string) string { return vmfiles.File(sto.ConfigRoot(), name) }

func (sto *Store) LoadVM(cfg schema.AppConfig) (vmoptions.Config, error) {
	if err := safeName(cfg.AppNameID); err != nil {
		return vmoptions.Config{}, err
	}
	options, err := vmfiles.Load(sto.ConfigRoot(), cfg.AppNameID)
	if err != nil {
		return options, fmt.Errorf("VM options %s: %w", sto.VMPath(cfg.AppNameID), err)
	}
	if options.AppNameID != cfg.AppNameID || options.Image != cfg.ImageMeta.Image {
		return options, fmt.Errorf("VM options %s do not match this app's identity/image", sto.VMPath(cfg.AppNameID))
	}
	return options, vmoptions.Validate(options)
}

// SaveDefinition keeps shared configuration and backend options together. Existing
// VM options are only replaced if the form actually changed them.
func (sto *Store) SaveDefinition(cfg schema.AppConfig, options *vmoptions.Config, creating bool) error {
	if creating {
		return sto.Create(cfg, options)
	}
	if options == nil {
		return sto.Save(cfg)
	}
	if err := checkDefinition(cfg, options); err != nil {
		return err
	}
	old, err := sto.Load(cfg.AppNameID)
	if err != nil {
		return err
	}
	if old.Type != schema.ZincVirtualization {
		return sto.adoptVM(cfg, *options)
	}
	previous, err := sto.LoadVM(old)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(previous, *options) {
		return sto.Save(cfg)
	}
	path := sto.VMPath(cfg.AppNameID)
	before, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(sto.Root, ".edit-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := vmfiles.Save(stage, *options); err != nil {
		return err
	}
	staged := vmfiles.File(stage, cfg.AppNameID)
	after, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	if err := replaceIfEqual(path, before, staged); err != nil {
		return err
	}
	if err := sto.Save(cfg); err != nil {
		rollback := filepath.Join(stage, "previous.json")
		if restoreErr := os.WriteFile(rollback, before, 0o600); restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
		return errors.Join(err, replaceIfEqual(path, after, rollback))
	}
	return nil
}

func replaceIfEqual(path string, expected []byte, staged string) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, expected) {
		return fmt.Errorf("%s changed concurrently; reload before saving", path)
	}
	return os.Rename(staged, path)
}

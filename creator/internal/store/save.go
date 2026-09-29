package store

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

// Save cannot reconstruct which keys an inherited document actually stated.
func (sto *Store) Save(cfg schema.AppConfig) error {
	if base := strings.TrimSpace(cfg.Inherits); base != "" {
		return fmt.Errorf("store: %s inherits from %q; edit its sparse YAML file: %s (a decoded form would replace inherited values with zeros)", cfg.AppNameID, base, sto.Path(cfg.AppNameID))
	}
	if err := validate.Validate(cfg); err != nil {
		return fmt.Errorf("store: refusing to save invalid config: %w", err)
	}
	data, err := Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(sto.Root, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(sto.Root, cfg.AppNameID+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := temporary.Sync(); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporary.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), sto.Path(cfg.AppNameID))
}

// CheckUnchanged detects edits made while a form was open. Runtime options and
// app data are compared separately, without regenerating either document.
func (sto *Store) CheckUnchanged(original schema.AppConfig, options *vmoptions.Config) error {
	current, err := sto.Load(original.AppNameID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, original) {
		return fmt.Errorf("%s changed while editing; reload before saving", original.AppNameID)
	}
	if options == nil {
		return nil
	}
	actual, err := sto.LoadVM(current)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, *options) {
		return fmt.Errorf("VM runtime options changed while editing; reload before saving")
	}
	return nil
}

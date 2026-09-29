package store

import (
	"errors"
	"os"

	vmfiles "github.com/crispuscrew/zinc/common/adapters/vmoptions"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

// A deliberate container-to-VM edit may create settings, but cannot replace an
// orphaned or independently authored settings file with the same name.
func (sto *Store) adoptVM(cfg schema.AppConfig, options vmoptions.Config) error {
	stage, err := os.MkdirTemp(sto.Root, ".vm-edit-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := vmfiles.Save(stage, options); err != nil {
		return err
	}
	source, destination := vmfiles.File(stage, cfg.AppNameID), sto.VMPath(cfg.AppNameID)
	if err := publish(source, destination); err != nil {
		return err
	}
	if err := sto.Save(cfg); err != nil {
		return errors.Join(err, removePublished(source, destination))
	}
	return nil
}

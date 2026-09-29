package store

import (
	"fmt"
	"os"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/inherit"
)

// Editing reads the sparse document; runtime dispatch reads the resolved one.
func (sto *Store) LoadResolved(name string) (schema.AppConfig, error) {
	data, err := sto.readRaw(name)
	if err != nil {
		return schema.AppConfig{}, err
	}
	merged, err := inherit.Resolve(data, sto.readRaw)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: %s: %w", name, err)
	}
	resolved, err := decode(merged, sto.Path(name))
	if err != nil {
		return schema.AppConfig{}, err
	}
	if resolved.AppNameID != name {
		return schema.AppConfig{}, fmt.Errorf("config: %s: resolves to AppNameID %q - an app must keep its own name; state AppNameID rather than taking the base's", name, resolved.AppNameID)
	}
	return resolved, nil
}

func (sto *Store) LoadFileResolved(path string) (schema.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	merged, err := inherit.Resolve(data, sto.readRaw)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return decode(merged, path)
}

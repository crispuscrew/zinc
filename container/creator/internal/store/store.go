// Package store persists app definitions as <name>.yaml files under the user's config
// directory (~/.config/zinc/apps) and provides the YAML decode/encode used by the
// $EDITOR round-trip.
//
// It is the creator's own copy of the on-disk format - deliberately independent of the
// runner so zcc never imports zcr (zcc authors app files; zcr runs them). Both sides
// read/write the exact same layout: the shared schema (common) plus this identical
// atomic-write + KnownFields codec, so a file zcc writes is one zcr loads verbatim.
//
// Save validates (validate.Validate) before writing, so invalid config never lands on
// disk, and writes are atomic (temp file + rename) so a crash can't leave a
// half-written definition. Load only decodes - zcr runs validate.Validate again at
// launch time, which catches drift from hand edits (docs/architecture.md section 3).
package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
)

// Load reads and decodes an app YAML from disk. It does NOT apply semantic rules -
// call validate.Validate on the result. Unknown keys (typos, stale fields after a hand
// edit) are reported as an error so dead config can't silently accumulate.
func Load(path string) (schema.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg schema.AppConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // reject unknown keys so stale/typo fields can't accumulate
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return schema.AppConfig{}, fmt.Errorf("config: %s: empty file", path)
		}
		return schema.AppConfig{}, fmt.Errorf("config: decode %s: %w", path, err)
	}
	return cfg, nil
}

// Marshal encodes an app config back to YAML - used to hand a draft to $EDITOR (the
// "advanced" form action) and round-trip it back through Load.
func Marshal(cfg schema.AppConfig) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("config: encode: %w", err)
	}
	return data, nil
}

// Store is a directory of app definitions.
type Store struct{ Root string }

// Default resolves the standard apps directory: $XDG_CONFIG_HOME/zinc/apps, falling
// back to ~/.config/zinc/apps.
func Default() (*Store, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("store: locate home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return &Store{Root: filepath.Join(base, "zinc", "apps")}, nil
}

// Path is the on-disk location of the named app's definition.
func (sto *Store) Path(name string) string {
	return filepath.Join(sto.Root, name+".yaml")
}

// List returns the names of all defined apps, sorted. A missing store directory is
// treated as empty, not an error.
func (sto *Store) List() ([]string, error) {
	entries, err := os.ReadDir(sto.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", sto.Root, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if name, ok := strings.CutSuffix(entry.Name(), ".yaml"); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// Exists reports whether an app with the given name is defined.
func (sto *Store) Exists(name string) bool {
	_, err := os.Stat(sto.Path(name))
	return err == nil
}

// Load decodes the named app. It does NOT validate - validate.Validate runs before
// launching (zcr) and before saving (below), which is what catches drift from hand
// edits (section 3).
func (sto *Store) Load(name string) (schema.AppConfig, error) {
	return Load(sto.Path(name))
}

// LoadFile decodes an arbitrary .yaml path (a CLI path argument, or the editor
// round-trip temp file) - same codec as Load, no store lookup.
func (sto *Store) LoadFile(path string) (schema.AppConfig, error) {
	return Load(path)
}

// Marshal encodes a draft to YAML for the $EDITOR round-trip (see the package Marshal
// function).
func (sto *Store) Marshal(cfg schema.AppConfig) ([]byte, error) {
	return Marshal(cfg)
}

// Save validates cfg and atomically writes it to <cfg.AppNameID>.yaml. Invalid config
// is rejected before anything touches disk.
func (sto *Store) Save(cfg schema.AppConfig) error {
	if err := validate.Validate(cfg); err != nil {
		return fmt.Errorf("store: refusing to save invalid config: %w", err)
	}
	if err := os.MkdirAll(sto.Root, 0o700); err != nil {
		return fmt.Errorf("store: create %s: %w", sto.Root, err)
	}

	data, err := Marshal(cfg)
	if err != nil {
		return fmt.Errorf("store: encode %s: %w", cfg.AppNameID, err)
	}

	tmp, err := os.CreateTemp(sto.Root, cfg.AppNameID+".*.tmp")
	if err != nil {
		return fmt.Errorf("store: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has succeeded

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("store: write %s: %w", cfg.AppNameID, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: write %s: %w", cfg.AppNameID, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("store: chmod %s: %w", cfg.AppNameID, err)
	}
	if err := os.Rename(tmpName, sto.Path(cfg.AppNameID)); err != nil {
		return fmt.Errorf("store: install %s: %w", cfg.AppNameID, err)
	}
	return nil
}

// Delete removes the named app definition. A missing definition is not an error.
func (sto *Store) Delete(name string) error {
	err := os.Remove(sto.Path(name))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: delete %s: %w", name, err)
	}
	return nil
}

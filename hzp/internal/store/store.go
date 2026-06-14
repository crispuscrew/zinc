// Package store persists app definitions as <name>.toml files under the user's
// config directory (~/.config/hyprzinc/apps).
//
// It is the imperative shell around the config functional core: Save validates
// before writing, so invalid config never lands on disk, and writes are atomic
// (temp file + rename) so a crash can't leave a half-written definition. Load
// only decodes — callers run config.Validate at launch time, which is what
// catches drift from hand edits (docs/architecture.md §3). hzp is the sole writer
// of these files; Nix only seeds them on first activation (§9.3).
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
)

// Store is a directory of app definitions.
type Store struct{ Root string }

// Default resolves the standard apps directory: $XDG_CONFIG_HOME/hyprzinc/apps,
// falling back to ~/.config/hyprzinc/apps.
func Default() (*Store, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("store: locate home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return &Store{Root: filepath.Join(base, "hyprzinc", "apps")}, nil
}

// Path is the on-disk location of the named app's definition.
func (s *Store) Path(name string) string {
	return filepath.Join(s.Root, name+".toml")
}

// List returns the names of all defined apps, sorted. A missing store directory
// is treated as empty, not an error.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", s.Root, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if n, ok := strings.CutSuffix(e.Name(), ".toml"); ok {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names, nil
}

// Exists reports whether an app with the given name is defined.
func (s *Store) Exists(name string) bool {
	_, err := os.Stat(s.Path(name))
	return err == nil
}

// Load decodes the named app. It does NOT validate — call config.Validate before
// launching, which is what catches drift from hand edits (§3).
func (s *Store) Load(name string) (config.AppConfig, error) {
	return config.Load(s.Path(name))
}

// Save validates cfg and atomically writes it to <cfg.App.Name>.toml. Invalid
// config is rejected before anything touches disk.
func (s *Store) Save(cfg config.AppConfig) error {
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("store: refusing to save invalid config: %w", err)
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return fmt.Errorf("store: create %s: %w", s.Root, err)
	}

	tmp, err := os.CreateTemp(s.Root, cfg.App.Name+".*.tmp")
	if err != nil {
		return fmt.Errorf("store: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has succeeded

	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("store: encode %s: %w", cfg.App.Name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: write %s: %w", cfg.App.Name, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("store: chmod %s: %w", cfg.App.Name, err)
	}
	if err := os.Rename(tmpName, s.Path(cfg.App.Name)); err != nil {
		return fmt.Errorf("store: install %s: %w", cfg.App.Name, err)
	}
	return nil
}

// Delete removes the named app definition. A missing definition is not an error.
func (s *Store) Delete(name string) error {
	err := os.Remove(s.Path(name))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: delete %s: %w", name, err)
	}
	return nil
}

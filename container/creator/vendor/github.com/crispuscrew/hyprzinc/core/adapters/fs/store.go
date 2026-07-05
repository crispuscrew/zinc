// Package fs is the filesystem adapter for the Store port: it persists app
// definitions as <name>.toml files under the user's config directory
// (~/.config/hyprzinc/apps) and provides the TOML decode/encode used by hzc's
// editor round-trip.
//
// Save validates (domain.Validate) before writing, so invalid config never lands on
// disk, and writes are atomic (temp file + rename) so a crash can't leave a
// half-written definition. Load only decodes — callers run domain.Validate at
// launch time, which catches drift from hand edits (docs/architecture.md §3). hzc
// is the sole writer; Nix only seeds these on first activation (§9.3).
package fs

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/core/ports"
)

// Compile-time check that the filesystem store satisfies the Store port.
var _ ports.Store = (*Store)(nil)

// Load reads and decodes an app TOML from disk. It does NOT apply semantic rules —
// call domain.Validate on the result. Unknown keys (typos, stale fields after a
// hand edit) are reported as an error so dead config can't silently accumulate.
func Load(path string) (domain.AppConfig, error) {
	var cfg domain.AppConfig
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return domain.AppConfig{}, fmt.Errorf("config: decode %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("config: %s: unknown key(s): %s", path, joinKeys(undecoded))
	}
	return cfg, nil
}

// Marshal encodes an app config back to TOML — used to hand a draft to $EDITOR (the
// "advanced" form action) and round-trip it back through Load.
func Marshal(cfg domain.AppConfig) ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return nil, fmt.Errorf("config: encode: %w", err)
	}
	return buf.Bytes(), nil
}

func joinKeys(keys []toml.Key) string {
	parts := make([]string, len(keys))
	for index, key := range keys {
		parts[index] = key.String()
	}
	return strings.Join(parts, ", ")
}

// Store is a directory of app definitions. It satisfies ports.Store.
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
func (sto *Store) Path(name string) string {
	return filepath.Join(sto.Root, name+".toml")
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
		if name, ok := strings.CutSuffix(entry.Name(), ".toml"); ok {
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

// Load decodes the named app. It does NOT validate — call domain.Validate before
// launching, which is what catches drift from hand edits (§3).
func (sto *Store) Load(name string) (domain.AppConfig, error) {
	return Load(sto.Path(name))
}

// LoadFile decodes an arbitrary .toml path (a CLI path argument, or the editor
// round-trip temp file) — same codec as Load, no store lookup.
func (sto *Store) LoadFile(path string) (domain.AppConfig, error) {
	return Load(path)
}

// Marshal encodes a draft to TOML for the $EDITOR round-trip (see the package
// Marshal function).
func (sto *Store) Marshal(cfg domain.AppConfig) ([]byte, error) {
	return Marshal(cfg)
}

// Save validates cfg and atomically writes it to <cfg.App.Name>.toml. Invalid
// config is rejected before anything touches disk.
func (sto *Store) Save(cfg domain.AppConfig) error {
	if err := domain.Validate(cfg); err != nil {
		return fmt.Errorf("store: refusing to save invalid config: %w", err)
	}
	if err := os.MkdirAll(sto.Root, 0o700); err != nil {
		return fmt.Errorf("store: create %s: %w", sto.Root, err)
	}

	tmp, err := os.CreateTemp(sto.Root, cfg.App.Name+".*.tmp")
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
	if err := os.Rename(tmpName, sto.Path(cfg.App.Name)); err != nil {
		return fmt.Errorf("store: install %s: %w", cfg.App.Name, err)
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

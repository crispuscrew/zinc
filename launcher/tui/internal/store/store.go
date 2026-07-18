// Package store reads app definitions from the shared config directory
// (~/.config/zinc/apps) - the same <name>.yaml files zcc writes and zcr runs. The
// launcher only ever reads: it lists the defined apps and loads them for display, then
// hands the chosen app to zcr to run. There is no write side here.
//
// It decodes the exact same layout as the other tools (the shared schema in common plus
// a KnownFields YAML codec), so a file zcc wrote is one zlt lists verbatim.
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
)

// Load reads and decodes an app YAML from disk. It does NOT apply semantic rules (zcr
// validates at launch time). Unknown keys are reported as an error so a stale/typo
// field surfaces rather than being silently ignored.
func Load(path string) (schema.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg schema.AppConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return schema.AppConfig{}, fmt.Errorf("config: %s: empty file", path)
		}
		return schema.AppConfig{}, fmt.Errorf("config: decode %s: %w", path, err)
	}
	return cfg, nil
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

// safeName rejects a name that is not a plain store key - one with a path separator or a
// ".." segment - so a name from the CLI cannot read a file outside the apps directory.
func safeName(name string) error {
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return fmt.Errorf("store: invalid app name %q", name)
	}
	return nil
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

// Load decodes the named app. The name must be a plain store key (safeName), so it
// cannot read a file outside the apps directory.
func (sto *Store) Load(name string) (schema.AppConfig, error) {
	if err := safeName(name); err != nil {
		return schema.AppConfig{}, err
	}
	return Load(sto.Path(name))
}

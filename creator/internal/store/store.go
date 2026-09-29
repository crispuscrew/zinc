// Package store persists app YAML and coordinates its host-local runtime options.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

type Store struct{ Root string }

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

func (sto *Store) Path(name string) string { return filepath.Join(sto.Root, name+".yaml") }

var keyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func safeName(name string) error {
	if !keyRE.MatchString(name) {
		return fmt.Errorf("store: invalid app name %q", name)
	}
	return nil
}

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
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-len(".yaml")]
		if keyRE.MatchString(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

func (sto *Store) Exists(name string) bool {
	if safeName(name) != nil {
		return false
	}
	_, err := os.Stat(sto.Path(name))
	return err == nil
}

func (sto *Store) readRaw(name string) ([]byte, error) {
	if err := safeName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(sto.Path(name))
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", name, err)
	}
	return data, nil
}

// Delete removes the app definition, retaining host-local VM settings and disks.
func (sto *Store) Delete(name string) error {
	if err := safeName(name); err != nil {
		return err
	}
	if err := os.Remove(sto.Path(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Package fs reads app definitions from the shared store (~/.config/zinc/apps). zvr only
// ever reads: authoring belongs to the creator, and a runner that could rewrite a config
// could quietly change what it is about to run.
package fs

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
	"github.com/crispuscrew/zinc/common/domain/schema/inherit"
)

// Store is a directory of app definitions, shared with the container tools: one store
// holds both app types and each runner takes the ones it owns.
type Store struct{ Root string }

// Default resolves the standard apps directory: $XDG_CONFIG_HOME/zinc/apps, falling back
// to ~/.config/zinc/apps.
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
func (store *Store) Path(name string) string {
	return filepath.Join(store.Root, name+".yaml")
}

// safeName rejects anything that is not a plain store key, so a crafted name cannot
// escape the apps directory when joined into Path. The check is per SEGMENT rather than
// a substring search for "..": a name like "my..app" contains those characters without
// being a traversal, and rejecting it would make an app that lists but cannot be loaded.
func safeName(name string) error {
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) {
		return fmt.Errorf("store: invalid app name %q", name)
	}
	return nil
}

// List returns the names of all defined apps, sorted. A missing store directory is
// treated as empty, not an error.
func (store *Store) List() ([]string, error) {
	entries, err := os.ReadDir(store.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", store.Root, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if name, found := strings.CutSuffix(entry.Name(), ".yaml"); found {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// Exists reports whether an app is defined. An unsafe name is treated as not-defined
// rather than stat'd through a traversal path.
func (store *Store) Exists(name string) bool {
	if safeName(name) != nil {
		return false
	}
	_, err := os.Stat(store.Path(name))
	return err == nil
}

// Load decodes the named app. It does not apply the schema rules - the caller runs
// validate.Validate before launching, which is what catches drift from a hand edit.
func (store *Store) Load(name string) (schema.AppConfig, error) {
	if err := safeName(name); err != nil {
		return schema.AppConfig{}, err
	}
	return LoadFile(store.Path(name))
}

// readRaw returns the named app's file as written, before any decoding. Inheritance is
// resolved on the YAML rather than on decoded structs - only the bytes record which keys the
// app actually STATED, and a decoded false is indistinguishable from an absent field.
func (store *Store) readRaw(name string) ([]byte, error) {
	if err := safeName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(store.Path(name))
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", name, err)
	}
	return data, nil
}

// LoadResolved decodes the named app with its Inherits chain applied: what the app does not
// state is taken from the base it starts from. This is what a launch reads, because it is
// what the app actually is - Load returns the file as written, which is what an editor needs
// and what must be written back.
func (store *Store) LoadResolved(name string) (schema.AppConfig, error) {
	data, err := store.readRaw(name)
	if err != nil {
		return schema.AppConfig{}, err
	}
	merged, err := inherit.Resolve(data, store.readRaw)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: %s: %w", name, err)
	}
	return decode(merged, store.Path(name))
}

// LoadFileResolved decodes an app YAML at an arbitrary path with its Inherits chain applied.
// The base is still looked up in the store: a config given by path is being read as an app,
// and where its base lives does not change because of how the app itself was named.
func (store *Store) LoadFileResolved(path string) (schema.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	merged, err := inherit.Resolve(data, store.readRaw)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return decode(merged, path)
}

// LoadFile decodes an app YAML at an arbitrary path, for a CLI path argument. Unknown
// keys are an error so a typo or a stale field cannot sit in a config doing nothing.
func LoadFile(path string) (schema.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	return decode(data, path)
}

// decode turns app YAML into a config. Unknown keys (typos, stale fields after a hand edit)
// are reported as an error so dead config can't silently accumulate. origin names the file
// for the error message; a merged config still names the app it was read for.
func decode(data []byte, origin string) (schema.AppConfig, error) {
	var cfg schema.AppConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return schema.AppConfig{}, fmt.Errorf("config: %s: empty file", origin)
		}
		return schema.AppConfig{}, fmt.Errorf("config: decode %s: %w", origin, err)
	}
	return cfg, nil
}

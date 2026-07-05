package keys

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Store reads and writes hzc's keybind selection under a config directory
// (~/.config/hyprzinc/hzc). It is the imperative shell around the pure scheme
// data; like core/store it validates before writing and writes atomically.
//
// Layout:
//
//	<Dir>/keys.toml            scheme = "<active-name>"
//	<Dir>/schemes/<name>.toml  base = "default|vim|…"  + [bindings.<ctx>] overrides
type Store struct{ Dir string }

// Active is the chosen scheme: its name (for display) and its merged bindings.
type Active struct {
	Name   string
	Scheme Scheme
}

// activeToml is the on-disk shape of keys.toml.
type activeToml struct {
	Scheme string `toml:"scheme"`
}

// customToml is the on-disk shape of a schemes/<name>.toml file. Bindings is
// context-name → action-name → keys; the names are validated on merge.
type customToml struct {
	Base     string                         `toml:"base"`
	Bindings map[string]map[string][]string `toml:"bindings"`
}

// DefaultStore resolves the standard config dir: $XDG_CONFIG_HOME/hyprzinc/hzc,
// falling back to ~/.config/hyprzinc/hzc.
func DefaultStore() (*Store, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("keys: locate home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return &Store{Dir: filepath.Join(base, "hyprzinc", "hzc")}, nil
}

func (s *Store) activePath() string         { return filepath.Join(s.Dir, "keys.toml") }
func (s *Store) schemesDir() string         { return filepath.Join(s.Dir, "schemes") }
func (s *Store) schemePath(n string) string { return filepath.Join(s.schemesDir(), n+".toml") }

// Load returns the active scheme. A missing keys.toml means "default". A bad
// active selection or custom scheme returns an error — the caller decides
// whether to fall back (main does, with a warning, so the TUI always starts).
func (s *Store) Load() (Active, error) {
	name, err := s.activeName()
	if err != nil {
		return Active{}, err
	}
	sc, err := s.resolve(name, map[string]bool{})
	if err != nil {
		return Active{}, err
	}
	return Active{Name: name, Scheme: sc}, nil
}

// activeName reads keys.toml's scheme field, defaulting to "default".
func (s *Store) activeName() (string, error) {
	var af activeToml
	_, err := toml.DecodeFile(s.activePath(), &af)
	if errors.Is(err, fs.ErrNotExist) {
		return "default", nil
	}
	if err != nil {
		return "", fmt.Errorf("keys: read %s: %w", s.activePath(), err)
	}
	if strings.TrimSpace(af.Scheme) == "" {
		return "default", nil
	}
	return af.Scheme, nil
}

// resolve produces the merged scheme for name: a built-in directly, or a custom
// file merged onto its (recursively resolved) base. seen guards against base
// cycles.
func (s *Store) resolve(name string, seen map[string]bool) (Scheme, error) {
	if sc, ok := SchemeFor(name); ok {
		return sc, nil
	}
	if seen[name] {
		return nil, fmt.Errorf("keys: scheme %q has a circular base", name)
	}
	seen[name] = true

	cf, err := readCustom(s.schemePath(name))
	if err != nil {
		return nil, fmt.Errorf("keys: scheme %q: %w", name, err)
	}
	base := cf.Base
	if base == "" {
		base = "default"
	}
	merged, err := s.resolve(base, seen)
	if err != nil {
		return nil, fmt.Errorf("keys: scheme %q base %q: %w", name, base, err)
	}
	merged = merged.clone()
	if err := applyOverrides(merged, cf.Bindings); err != nil {
		return nil, fmt.Errorf("keys: scheme %q: %w", name, err)
	}
	if err := Validate(merged); err != nil {
		return nil, fmt.Errorf("keys: scheme %q: %w", name, err)
	}
	return merged, nil
}

// Resolve returns the merged scheme for an arbitrary name (built-in or custom),
// for `hzc keys show <name>` and for validation.
func (s *Store) Resolve(name string) (Scheme, error) {
	return s.resolve(name, map[string]bool{})
}

// Validate resolves a scheme and reports any problem; nil means it is usable.
func (s *Store) Validate(name string) error {
	_, err := s.resolve(name, map[string]bool{})
	return err
}

// SetActive writes keys.toml after confirming name resolves to a valid scheme.
func (s *Store) SetActive(name string) error {
	if _, err := s.resolve(name, map[string]bool{}); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("keys: create %s: %w", s.Dir, err)
	}
	tmp, err := os.CreateTemp(s.Dir, "keys.*.tmp")
	if err != nil {
		return fmt.Errorf("keys: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if err := toml.NewEncoder(tmp).Encode(activeToml{Scheme: name}); err != nil {
		tmp.Close()
		return fmt.Errorf("keys: encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("keys: write: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("keys: chmod: %w", err)
	}
	if err := os.Rename(tmpName, s.activePath()); err != nil {
		return fmt.Errorf("keys: install: %w", err)
	}
	return nil
}

// List returns the selectable scheme names: the built-ins first, then any
// custom files under schemes/, sorted. A missing schemes/ dir is not an error.
func (s *Store) List() ([]string, error) {
	names := BuiltinNames()
	entries, err := os.ReadDir(s.schemesDir())
	if errors.Is(err, fs.ErrNotExist) {
		return names, nil
	}
	if err != nil {
		return nil, fmt.Errorf("keys: read %s: %w", s.schemesDir(), err)
	}
	var custom []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if n, ok := strings.CutSuffix(e.Name(), ".toml"); ok && !IsBuiltin(n) {
			custom = append(custom, n)
		}
	}
	slices.Sort(custom)
	return append(names, custom...), nil
}

// EnsureEditable returns the path to an editable custom scheme file, creating a
// commented starter if it doesn't exist, and the scheme name it can be set to.
// Editing a built-in scaffolds "<name>-custom" (a custom file may not shadow a
// built-in, since resolve matches built-ins first).
func (s *Store) EnsureEditable(name string) (schemeName, path string, err error) {
	target, base := name, "default"
	if IsBuiltin(name) {
		target, base = name+"-custom", name
	}
	path = s.schemePath(target)
	switch _, statErr := os.Stat(path); {
	case statErr == nil:
		return target, path, nil // already exists; just edit it
	case !errors.Is(statErr, fs.ErrNotExist):
		return "", "", statErr
	}
	if err := os.MkdirAll(s.schemesDir(), 0o700); err != nil {
		return "", "", fmt.Errorf("keys: create %s: %w", s.schemesDir(), err)
	}
	if err := os.WriteFile(path, []byte(template(target, base)), 0o600); err != nil {
		return "", "", fmt.Errorf("keys: write %s: %w", path, err)
	}
	return target, path, nil
}

// readCustom decodes a custom scheme file, surfacing a missing file and stray
// top-level keys (e.g. a "bas" typo for "base") as clear errors. Unknown action
// names live inside the bindings map and are caught later, on merge.
func readCustom(path string) (customToml, error) {
	var cf customToml
	meta, err := toml.DecodeFile(path, &cf)
	if errors.Is(err, fs.ErrNotExist) {
		return cf, fmt.Errorf("no such scheme (looked for %s)", path)
	}
	if err != nil {
		return cf, fmt.Errorf("parse %s: %w", path, err)
	}
	if und := meta.Undecoded(); len(und) > 0 {
		return cf, fmt.Errorf("unknown keys in %s: %v", path, und)
	}
	return cf, nil
}

// applyOverrides merges a custom file's bindings onto a base scheme, replacing
// an action's keys outright. Unknown context or action names are rejected
// (accumulated), so a typo fails loudly instead of silently doing nothing.
func applyOverrides(s Scheme, raw map[string]map[string][]string) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	for _, cname := range sortedStringKeys(raw) {
		ctx, ok := contextByName(cname)
		if !ok {
			add("[bindings.%s]: unknown screen", cname)
			continue
		}
		for _, aname := range sortedStringKeys(raw[cname]) {
			act := Action(aname)
			if !knownAction(ctx, act) {
				add("[bindings.%s] unknown action %q", cname, aname)
				continue
			}
			if s[ctx] == nil {
				s[ctx] = map[Action][]string{}
			}
			s[ctx][act] = append([]string(nil), raw[cname][aname]...)
		}
	}
	return errors.Join(errs...)
}

// template is the commented starter written for a new custom scheme.
func template(name, base string) string {
	return fmt.Sprintf(`# hzc keybind scheme %q — see "hzc keys show %s" for every action name.
# Inherits all bindings from %q; list only the keys you want to change.
base = %q

# [bindings.list]
# refresh = ["g", "ctrl+r"]
# quit    = ["q", "Q"]

# [bindings.form]
# save = ["ctrl+s"]
`, name, base, base, base)
}

func sortedStringKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

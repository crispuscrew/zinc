// Package vmoptions stores host-local VM options independently of app YAML.
package vmoptions

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	domain "github.com/crispuscrew/zinc/common/domain/vmoptions"
)

const maxBytes = 1 << 20

// File is pure. An invalid name returns no path; Load and Save return its error.
func File(configRoot, name string) string {
	if domain.Name(name) != nil || !filepath.IsAbs(configRoot) {
		return ""
	}
	return filepath.Join(configRoot, "zinc", "runtime", "vm", name+".json")
}

func Load(configRoot, name string) (domain.Config, error) {
	path := File(configRoot, name)
	if path == "" {
		return domain.Config{}, fmt.Errorf("VM options: invalid config root or app name")
	}
	data, err := readFile(path)
	if err != nil {
		return domain.Config{}, fmt.Errorf("VM options %s: %w", path, err)
	}
	config, err := decode(data)
	if err != nil {
		return domain.Config{}, fmt.Errorf("VM options %s: %w", path, err)
	}
	if config.AppNameID != name {
		return domain.Config{}, fmt.Errorf("VM options %s: AppNameID %q does not match %q", path, config.AppNameID, name)
	}
	return config, domain.Validate(config)
}

func readFile(path string) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("require a regular file no larger than %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if len(data) > maxBytes {
		return nil, fmt.Errorf("options exceed %d bytes", maxBytes)
	}
	return data, err
}

// Save never replaces different existing content. Callers must explicitly resolve
// that conflict rather than overwrite another author's work with a stale draft.
func Save(configRoot string, config domain.Config) error {
	if err := domain.Validate(config); err != nil {
		return err
	}
	path := File(configRoot, config.AppNameID)
	if path == "" {
		return fmt.Errorf("VM options: config root must be absolute")
	}
	data, err := encode(config)
	if err != nil {
		return err
	}
	if err := prepareDirs(configRoot); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".options-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	// Link publishes the complete 0600 file atomically, without replacing a winner.
	if err := os.Link(file.Name(), path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, loadErr := Load(configRoot, config.AppNameID)
		if loadErr == nil {
			prior, encodeErr := encode(existing)
			if encodeErr == nil && bytes.Equal(prior, data) {
				return nil
			}
		}
		return fmt.Errorf("VM options conflict at %s: existing content differs; explicit replacement is required", path)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

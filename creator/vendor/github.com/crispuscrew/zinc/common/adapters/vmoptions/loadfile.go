package vmoptions

import (
	"fmt"
	"path/filepath"

	domain "github.com/crispuscrew/zinc/common/domain/vmoptions"
)

// LoadFile is the explicit-path counterpart to Load; it never discovers files.
func LoadFile(path string) (domain.Config, error) {
	if !filepath.IsAbs(path) {
		return domain.Config{}, fmt.Errorf("VM runtime options path must be absolute")
	}
	data, err := readFile(path)
	if err != nil {
		return domain.Config{}, err
	}
	config, err := decode(data)
	if err != nil {
		return domain.Config{}, err
	}
	return config, domain.Validate(config)
}

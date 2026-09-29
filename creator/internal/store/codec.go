package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

func Load(path string) (schema.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	return decode(data, path)
}

func decode(data []byte, origin string) (schema.AppConfig, error) {
	migrated, err := schema.Migrate(data)
	if err != nil {
		return schema.AppConfig{}, fmt.Errorf("config: migrate %s: %w", origin, err)
	}
	var cfg schema.AppConfig
	decoder := yaml.NewDecoder(bytes.NewReader(migrated))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return cfg, fmt.Errorf("config: %s: empty file", origin)
		}
		return cfg, fmt.Errorf("config: decode %s: %w", origin, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return cfg, fmt.Errorf("config: %s: expected one YAML document", origin)
	}
	return cfg, nil
}

func Marshal(cfg schema.AppConfig) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("config: encode: %w", err)
	}
	return data, nil
}

func (sto *Store) Load(name string) (schema.AppConfig, error) {
	if err := safeName(name); err != nil {
		return schema.AppConfig{}, err
	}
	return Load(sto.Path(name))
}

func (sto *Store) LoadFile(path string) (schema.AppConfig, error) { return Load(path) }
func (sto *Store) Marshal(cfg schema.AppConfig) ([]byte, error)   { return Marshal(cfg) }

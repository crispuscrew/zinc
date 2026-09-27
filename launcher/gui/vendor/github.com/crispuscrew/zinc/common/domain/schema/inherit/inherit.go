// Package inherit resolves YAML app inheritance before decoding, preserving the
// distinction between an absent field and an explicit false, empty list or map.
package inherit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

const maxDepth = 8

// Parent names are joined into filesystem paths by stores.
var baseNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Parent reads just Inherits; an incomplete child need not be valid on its own.
func Parent(data []byte) (string, error) {
	var header struct {
		Inherits string `yaml:"Inherits"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return "", fmt.Errorf("reading Inherits: %w", err)
	}
	name := strings.TrimSpace(header.Inherits)
	if name == "" {
		return "", nil
	}
	if !baseNameRE.MatchString(name) {
		return "", fmt.Errorf("Inherits %q: only lowercase [a-z0-9._-] allowed, must start alphanumeric", name)
	}
	return name, nil
}

// Resolve migrates each member of the chain before overlaying ancestor to child.
// The result is for decoding, not for saving over the authored child document.
func Resolve(data []byte, loadBase func(name string) ([]byte, error)) ([]byte, error) {
	migrated, err := schema.Migrate(data)
	if err != nil {
		return nil, fmt.Errorf("migrate app: %w", err)
	}
	chain := [][]byte{migrated}
	seen := map[string]bool{}
	current := migrated
	for depth := 0; ; depth++ {
		parent, err := Parent(current)
		if err != nil {
			return nil, err
		}
		if parent == "" {
			break
		}
		if seen[parent] {
			return nil, fmt.Errorf("inheritance cycle: %q is already in the chain", parent)
		}
		if depth >= maxDepth {
			return nil, fmt.Errorf("inheritance chain deeper than %d - %q is where it was cut off", maxDepth, parent)
		}
		seen[parent] = true
		baseData, err := loadBase(parent)
		if err != nil {
			return nil, fmt.Errorf("inherits %q: %w", parent, err)
		}
		baseData, err = schema.Migrate(baseData)
		if err != nil {
			return nil, fmt.Errorf("inherits %q: migrate: %w", parent, err)
		}
		chain = append(chain, baseData)
		current = baseData
	}
	if len(chain) == 1 {
		return migrated, nil
	}
	merged := chain[len(chain)-1]
	for index := len(chain) - 2; index >= 0; index-- {
		next, err := Merge(merged, chain[index])
		if err != nil {
			return nil, err
		}
		merged = next
	}
	return merged, nil
}

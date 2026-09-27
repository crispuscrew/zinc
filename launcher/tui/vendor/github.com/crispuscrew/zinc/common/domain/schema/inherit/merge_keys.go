package inherit

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func expandMergeKeys(mapping *yaml.Node) error {
	if len(mapping.Content)%2 != 0 {
		return fmt.Errorf("incomplete YAML mapping at line %d", mapping.Line)
	}
	seen := map[string]bool{}
	present := map[string]bool{}
	var content, sources []*yaml.Node
	for index := 0; index < len(mapping.Content); index += 2 {
		key, value := mapping.Content[index], mapping.Content[index+1]
		if key.Kind != yaml.ScalarNode || (key.Tag != "!!str" && key.Tag != "!!merge") {
			return fmt.Errorf("YAML mapping key at line %d must be a string", key.Line)
		}
		if seen[key.Value] {
			return fmt.Errorf("duplicate YAML key %q at line %d", key.Value, key.Line)
		}
		seen[key.Value] = true
		if key.Tag != "!!merge" {
			content = append(content, key, value)
			present[key.Value] = true
			continue
		}
		if value.Kind == yaml.SequenceNode {
			sources = append(sources, value.Content...)
		} else {
			sources = append(sources, value)
		}
	}
	// Explicit keys win regardless of position. Among merge sources, the first
	// mapping containing a key wins; duplicates inside any one mapping are errors.
	for _, source := range sources {
		if source.Kind != yaml.MappingNode {
			return fmt.Errorf("YAML merge at line %d requires a mapping or a sequence of mappings", source.Line)
		}
		for index := 0; index < len(source.Content); index += 2 {
			key, value := source.Content[index], source.Content[index+1]
			if !present[key.Value] {
				content = append(content, key, value)
				present[key.Value] = true
			}
		}
	}
	mapping.Content = content
	return nil
}

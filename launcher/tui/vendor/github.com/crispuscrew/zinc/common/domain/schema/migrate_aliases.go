package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Snapshot aliases before editing their targets. Moving an anchored key/value can
// otherwise change an unrelated alias or emit a forward reference YAML cannot read.
func expandMigrationNode(node *yaml.Node, active map[*yaml.Node]bool) (*yaml.Node, error) {
	if active[node] {
		return nil, fmt.Errorf("recursive YAML alias at line %d", node.Line)
	}
	active[node] = true
	defer delete(active, node)
	if node.Kind == yaml.AliasNode {
		copy, err := expandMigrationNode(node.Alias, active)
		if err == nil {
			clearMigrationAnchors(copy)
		}
		return copy, err
	}
	copy := *node
	copy.Content = nil
	for _, child := range node.Content {
		expanded, err := expandMigrationNode(child, active)
		if err != nil {
			return nil, err
		}
		copy.Content = append(copy.Content, expanded)
	}
	if copy.Kind != yaml.MappingNode {
		return &copy, nil
	}
	seen := map[string]bool{}
	var merges []*yaml.Node
	var content []*yaml.Node
	for index := 0; index < len(copy.Content); index += 2 {
		key, value := copy.Content[index], copy.Content[index+1]
		if key.Kind != yaml.ScalarNode || (key.Tag != "!!str" && key.Tag != "!!merge") {
			return nil, fmt.Errorf("YAML mapping key at line %d must be a string", key.Line)
		}
		if seen[key.Value] {
			return nil, fmt.Errorf("duplicate YAML key %q at line %d", key.Value, key.Line)
		}
		seen[key.Value] = true
		if key.Tag == "!!merge" {
			if value.Kind == yaml.SequenceNode {
				merges = append(merges, value.Content...)
			} else {
				merges = append(merges, value)
			}
		} else {
			content = append(content, key, value)
		}
	}
	copy.Content = content
	for _, merged := range merges {
		if merged.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("YAML merge at line %d requires a mapping", merged.Line)
		}
		for index := 0; index < len(merged.Content); index += 2 {
			key := merged.Content[index]
			if !seen[key.Value] {
				copy.Content = append(copy.Content, key, merged.Content[index+1])
				seen[key.Value] = true
			}
		}
	}
	return &copy, nil
}

func clearMigrationAnchors(node *yaml.Node) {
	node.Anchor = ""
	for _, child := range node.Content {
		clearMigrationAnchors(child)
	}
}

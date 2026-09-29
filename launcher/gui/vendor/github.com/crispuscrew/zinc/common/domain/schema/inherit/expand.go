package inherit

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Bound the expanded document, including repeated scalar text, rather than just
// its compact source. Ordinary app configs are far smaller than these limits.
const (
	maxYAMLDepth = 128
	maxYAMLNodes = 100000
	maxYAMLBytes = 16 * 1024 * 1024
)

type yamlExpansion struct {
	active         map[*yaml.Node]bool
	remainingNodes int
	remainingBytes int
}

func expandYAML(root *yaml.Node) (*yaml.Node, error) {
	expansion := yamlExpansion{
		active: map[*yaml.Node]bool{}, remainingNodes: maxYAMLNodes, remainingBytes: maxYAMLBytes,
	}
	return expansion.expand(root, 0)
}

func (expansion *yamlExpansion) expand(node *yaml.Node, depth int) (*yaml.Node, error) {
	if node == nil {
		return nil, fmt.Errorf("YAML alias has no target")
	}
	if expansion.active[node] {
		return nil, fmt.Errorf("recursive YAML alias at line %d", node.Line)
	}
	if depth > maxYAMLDepth {
		return nil, fmt.Errorf("YAML expansion exceeds depth limit %d at line %d", maxYAMLDepth, node.Line)
	}
	expansion.remainingNodes--
	expansion.remainingBytes -= len(node.Value) + len(node.Tag) +
		len(node.HeadComment) + len(node.LineComment) + len(node.FootComment)
	if expansion.remainingNodes < 0 || expansion.remainingBytes < 0 {
		return nil, fmt.Errorf("YAML expansion exceeds size limit (%d nodes, %d text bytes)", maxYAMLNodes, maxYAMLBytes)
	}
	expansion.active[node] = true
	defer delete(expansion.active, node)
	if node.Kind == yaml.AliasNode {
		return expansion.expand(node.Alias, depth+1)
	}
	// Retain kind, tag, style and lexical value until the final typed decode. A
	// Go-value round trip here would turn an environment string like 0022 into 18.
	result := *node
	result.Anchor, result.Alias, result.Content = "", nil, nil
	for _, child := range node.Content {
		expanded, err := expansion.expand(child, depth+1)
		if err != nil {
			return nil, err
		}
		result.Content = append(result.Content, expanded)
	}
	if result.Kind == yaml.MappingNode {
		if err := expandMergeKeys(&result); err != nil {
			return nil, err
		}
	}
	return &result, nil
}

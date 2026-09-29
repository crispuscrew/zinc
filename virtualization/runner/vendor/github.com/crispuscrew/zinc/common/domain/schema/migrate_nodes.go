package schema

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func mappingValue(mapping *yaml.Node, name string) *yaml.Node {
	if index := mappingIndex(mapping, name); index >= 0 {
		return mapping.Content[index+1]
	}
	return nil
}

func mappingIndex(mapping *yaml.Node, name string) int {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return -1
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == name {
			return index
		}
	}
	return -1
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func mappingNode() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }

func (change *migration) value(path string) *yaml.Node {
	node := change.root
	for _, name := range strings.Split(path, ".") {
		node = mappingValue(node, name)
	}
	return node
}

func (change *migration) parent(path string) (*yaml.Node, string, error) {
	names := strings.Split(path, ".")
	parent := change.root
	for _, name := range names[:len(names)-1] {
		next := mappingValue(parent, name)
		if next == nil {
			next = mappingNode()
			parent.Content = append(parent.Content, scalarNode(name), next)
		} else if next.Tag == "!!null" {
			*next = *mappingNode()
		}
		if next.Kind != yaml.MappingNode {
			return nil, "", fmt.Errorf("%s: %s must be a mapping", path, name)
		}
		parent = next
	}
	return parent, names[len(names)-1], nil
}

func (change *migration) put(path string, value *yaml.Node) error {
	parent, name, err := change.parent(path)
	if err != nil {
		return err
	}
	if mappingIndex(parent, name) >= 0 {
		return fmt.Errorf("%s is already set; remove the legacy field or resolve the collision explicitly", path)
	}
	parent.Content = append(parent.Content, scalarNode(name), value)
	change.changed = true
	return nil
}

func (change *migration) remove(path string) {
	names := strings.Split(path, ".")
	parent := change.root
	for _, name := range names[:len(names)-1] {
		parent = mappingValue(parent, name)
	}
	if index := mappingIndex(parent, names[len(names)-1]); index >= 0 {
		parent.Content = append(parent.Content[:index], parent.Content[index+2:]...)
		change.changed = true
	}
}

func (change *migration) move(source, target string) error {
	value := change.value(source)
	if value == nil {
		return nil
	}
	if err := change.put(target, value); err != nil {
		return fmt.Errorf("%s -> %s: %w", source, target, err)
	}
	change.remove(source)
	return nil
}

func encodedNode(value any) (*yaml.Node, error) {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return nil, err
	}
	return &node, nil
}

func (change *migration) putValue(path string, value any) error {
	node, err := encodedNode(value)
	if err != nil {
		return err
	}
	return change.put(path, node)
}

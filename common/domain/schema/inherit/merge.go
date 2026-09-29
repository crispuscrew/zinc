package inherit

import (
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

// Merge overlays child onto base. Explicit lists, environment maps and audio
// directions replace the inherited value, allowing a child to revoke grants.
func Merge(base, child []byte) ([]byte, error) {
	baseNode, err := documentRoot(base)
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	childNode, err := documentRoot(child)
	if err != nil {
		return nil, fmt.Errorf("child: %w", err)
	}
	if baseNode == nil && childNode == nil {
		return nil, nil
	}
	if baseNode == nil {
		baseNode = childNode
	} else if childNode != nil {
		baseNode = mergeNode(baseNode, childNode, "")
	}
	data, err := yaml.Marshal(baseNode)
	if err != nil {
		return nil, fmt.Errorf("encoding the merged config: %w", err)
	}
	return data, nil
}

func documentRoot(data []byte) (*yaml.Node, error) {
	migrated, err := schema.Migrate(data)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(migrated, &document); err != nil {
		return nil, err
	}
	if len(document.Content) == 0 {
		return nil, nil
	}
	if document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("want a mapping at the top level")
	}
	// Resolve aliases within their original document before overriding an anchor
	// or combining two documents that happen to use the same anchor name.
	return expandYAML(document.Content[0])
}

var replaceWhole = map[string]bool{
	"StartConditions.EntrypointEnv": true,
	"StartConditions.AttachedEnv":   true,
	"AudioMeta.Playback":            true,
	"AudioMeta.Microphone":          true,
	"AudioMeta.Monitor":             true,
}

func mergeNode(base, child *yaml.Node, path string) *yaml.Node {
	if replaceWhole[path] || base.Kind != yaml.MappingNode || child.Kind != yaml.MappingNode {
		return child
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: base.Tag, Style: base.Style}
	out.Content = append(out.Content, base.Content...)
	for index := 0; index+1 < len(child.Content); index += 2 {
		key, value := child.Content[index], child.Content[index+1]
		position := findKey(out, key.Value)
		if position < 0 {
			out.Content = append(out.Content, key, value)
			continue
		}
		nested := key.Value
		if path != "" {
			nested = path + "." + key.Value
		}
		out.Content[position+1] = mergeNode(out.Content[position+1], value, nested)
	}
	return out
}

func findKey(mapping *yaml.Node, name string) int {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == name {
			return index
		}
	}
	return -1
}

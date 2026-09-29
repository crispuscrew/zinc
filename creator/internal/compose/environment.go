package compose

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Environment rejects implicit host lookups: importing a file must not copy host
// secrets just because a variable was written without an explicit value.
type Environment map[string]string

func (environment *Environment) UnmarshalYAML(node *yaml.Node) error {
	result := Environment{}
	add := func(name, value string) error {
		if name == "" {
			return fmt.Errorf("environment variable name must not be empty")
		}
		if _, exists := result[name]; exists {
			return fmt.Errorf("duplicate environment variable %q", name)
		}
		result[name] = value
		return nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		for index := 0; index < len(node.Content); index += 2 {
			value := node.Content[index+1]
			if value.Tag == "!!null" {
				return fmt.Errorf("environment %q requests host inheritance; provide an explicit value", node.Content[index].Value)
			}
			text, err := scalarText(value)
			if err != nil {
				return err
			}
			if err := add(node.Content[index].Value, text); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, value := range node.Content {
			text, err := scalarText(value)
			if err != nil {
				return err
			}
			name, content, found := strings.Cut(text, "=")
			if !found {
				return fmt.Errorf("environment %q requests host inheritance; provide NAME=VALUE", text)
			}
			if err := add(name, content); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("environment must be a mapping or NAME=VALUE list")
	}
	*environment = result
	return nil
}

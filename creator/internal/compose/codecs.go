package compose

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type Labels map[string]string

func (labels *Labels) UnmarshalYAML(node *yaml.Node) error {
	result := Labels{}
	switch node.Kind {
	case yaml.MappingNode:
		if err := node.Decode((*map[string]string)(&result)); err != nil {
			return err
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			text, err := scalarText(item)
			if err != nil {
				return err
			}
			key, value, _ := strings.Cut(text, "=")
			result[key] = value
		}
	default:
		return fmt.Errorf("line %d: labels want a mapping or key=value list", node.Line)
	}
	*labels = result
	return nil
}

type StringList []string

func (list *StringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		value, err := scalarText(node)
		if err != nil {
			return err
		}
		*list = StringList{value}
	case yaml.SequenceNode:
		result := make(StringList, 0, len(node.Content))
		for _, item := range node.Content {
			var text string
			var err error
			if item.Kind == yaml.MappingNode {
				text, err = longFormEntry(item)
			} else {
				text, err = scalarText(item)
			}
			if err != nil {
				return err
			}
			result = append(result, text)
		}
		*list = result
	default:
		return fmt.Errorf("line %d: want a string or string list", node.Line)
	}
	return nil
}

type Dependencies map[string]Depend

func (dependencies *Dependencies) UnmarshalYAML(node *yaml.Node) error {
	result := Dependencies{}
	switch node.Kind {
	case yaml.SequenceNode:
		for _, item := range node.Content {
			name, err := scalarText(item)
			if err != nil {
				return err
			}
			result[name] = Depend{Condition: ConditionStarted}
		}
	case yaml.MappingNode:
		if err := node.Decode((*map[string]Depend)(&result)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("line %d: depends_on wants a list or mapping", node.Line)
	}
	*dependencies = result
	return nil
}

func (dependencies Dependencies) Names() []string {
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

package schema

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var audioDirections = []string{"Playback", "Microphone", "Monitor"}

func (change *migration) migrateAudio() error {
	for _, direction := range audioDirections {
		path := "AudioMeta." + direction
		value := change.value(path)
		if value == nil || value.Tag == "!!null" || value.Kind == yaml.MappingNode {
			continue
		}
		replacement := mappingNode()
		switch {
		case value.Kind == yaml.ScalarNode && value.Tag == "!!str" && value.Value == "none":
		case value.Kind == yaml.ScalarNode && value.Tag == "!!str" && value.Value == "default":
			replacement.Content = append(replacement.Content, scalarNode("PipeWireDefault"),
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		case value.Kind == yaml.SequenceNode:
			for _, device := range value.Content {
				if device.Tag != "!!str" || !strings.HasPrefix(device.Value, "/dev/snd/") {
					return fmt.Errorf("%s: legacy lists must name /dev/snd/ devices; use PipeWireDevices or ALSADevices explicitly", path)
				}
			}
			replacement.Content = append(replacement.Content, scalarNode("ALSADevices"), value)
		default:
			return fmt.Errorf("%s: expected legacy none, default, or a /dev/snd/ list; use a structured audio mapping", path)
		}
		change.remove(path)
		if err := change.put(path, replacement); err != nil {
			return err
		}
	}
	return nil
}

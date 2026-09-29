package compose

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Refuse unsupported nested settings instead of flattening away a restriction.
func longFormEntry(node *yaml.Node) (string, error) {
	fields := map[string]string{}
	for index := 0; index+1 < len(node.Content); index += 2 {
		name := strings.ToLower(node.Content[index].Value)
		switch name {
		case "type", "source", "target", "read_only", "published", "host_ip", "protocol":
		default:
			return "", fmt.Errorf("line %d: long-form option %q is not representable", node.Line, name)
		}
		value, err := scalarText(node.Content[index+1])
		if err != nil {
			return "", err
		}
		if _, exists := fields[name]; exists {
			return "", fmt.Errorf("line %d: duplicate option %q", node.Line, name)
		}
		fields[name] = value
	}
	if fields["target"] == "" {
		return "", fmt.Errorf("line %d: long-form entry requires target", node.Line)
	}
	if fields["type"] != "" {
		if fields["type"] != "bind" && fields["type"] != "volume" {
			return "", fmt.Errorf("mount type %q is not representable", fields["type"])
		}
		mount := fields["source"] + ":" + fields["target"]
		if fields["read_only"] == "true" {
			mount += ":ro"
		}
		return mount, nil
	}
	port := fields["target"]
	if published := fields["published"]; published != "" {
		port = published + ":" + port
	}
	if host := fields["host_ip"]; host != "" {
		if fields["published"] == "" {
			return "", fmt.Errorf("host_ip without a published port is not representable")
		}
		port = host + ":" + port
	}
	if protocol := fields["protocol"]; protocol != "" {
		port += "/" + protocol
	}
	return port, nil
}

func scalarText(node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("line %d: want a scalar", node.Line)
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return "", err
	}
	switch value := value.(type) {
	case string:
		return value, nil
	case int:
		return strconv.Itoa(value), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(value), nil
	default:
		return "", fmt.Errorf("line %d: want a scalar value", node.Line)
	}
}

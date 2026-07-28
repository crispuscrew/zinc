// Package compose translates between a Zinc app definition and a Compose-specification
// file, in both directions. It lives in the creator rather than in common because both
// directions are authoring: exporting describes an app in the format other tools read,
// and importing turns someone else's compose file into app definitions. Nothing here runs
// anything, and the runner never sees a compose file.
//
// The two directions are not inverses, and neither is lossless:
//
//   - Exporting drops guarantees. A compose file cannot express the nftables egress
//     lock-down applied to the app's netns before it starts, the Wayland security context,
//     or the pod ordering the runner performs - so an exported file DESCRIBES an app, and
//     running it with podman-compose gives a container with less containment than `zcr
//     run` gives. Everything dropped is returned as a note rather than silently omitted.
//
//   - Importing tightens. Compose says nothing about egress, so an imported app gets
//     Zinc's default - no NetworkLists, which is no network at all - regardless of how
//     open the compose service was. Published ports are the one exception, because they
//     are stated outright.
//
// The model below is the subset of the Compose specification that carries meaning for a
// Zinc app. Fields Zinc has no use for are not modelled, so importing simply ignores them;
// what is ignored and mattered is reported as a note.
package compose

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Project is a compose file: the services it defines, plus the networks they join.
type Project struct {
	Name     string             `yaml:"name,omitempty"`
	Services map[string]Service `yaml:"services"`
	Networks map[string]Network `yaml:"networks,omitempty"`
}

// Service is one container definition.
type Service struct {
	Image         string       `yaml:"image,omitempty"`
	ContainerName string       `yaml:"container_name,omitempty"`
	Entrypoint    StringList   `yaml:"entrypoint,omitempty"`
	Command       StringList   `yaml:"command,omitempty"`
	User          string       `yaml:"user,omitempty"`
	Restart       string       `yaml:"restart,omitempty"`
	CapDrop       []string     `yaml:"cap_drop,omitempty"`
	CapAdd        []string     `yaml:"cap_add,omitempty"`
	SecurityOpt   []string     `yaml:"security_opt,omitempty"`
	PidsLimit     int64        `yaml:"pids_limit,omitempty"`
	Ports         StringList   `yaml:"ports,omitempty"`
	Expose        StringList   `yaml:"expose,omitempty"`
	Volumes       StringList   `yaml:"volumes,omitempty"`
	Networks      StringList   `yaml:"networks,omitempty"`
	DependsOn     Dependencies `yaml:"depends_on,omitempty"`
	Healthcheck   *Healthcheck `yaml:"healthcheck,omitempty"`
	Deploy        *Deploy      `yaml:"deploy,omitempty"`
	Labels        Labels       `yaml:"labels,omitempty"`
	DNS           StringList   `yaml:"dns,omitempty"`
}

// Network is a compose network. Only Internal carries meaning here: it is the compose
// spelling of a bridge that reaches no further than the services on it, which is what a
// Zinc sibling link is.
type Network struct {
	Internal bool `yaml:"internal,omitempty"`
}

// Depend is one entry of the long form of depends_on. Condition is compose's
// service_started / service_healthy: the same distinction as a Zinc dependency with or
// without a ReadyCheck.
type Depend struct {
	Condition string `yaml:"condition,omitempty"`
}

// Compose's two dependency conditions. service_healthy is the one that waits.
const (
	ConditionStarted = "service_started"
	ConditionHealthy = "service_healthy"
)

// Healthcheck is compose's readiness probe. Test is the docker convention: a leading
// "CMD" means run the rest as argv, "CMD-SHELL" means run one string through a shell,
// "NONE" disables an inherited check.
type Healthcheck struct {
	Test     StringList `yaml:"test,omitempty"`
	Interval string     `yaml:"interval,omitempty"`
	Timeout  string     `yaml:"timeout,omitempty"`
	Retries  int        `yaml:"retries,omitempty"`
}

// Deploy carries the resource limits. compose nests them this deep, and podman-compose
// reads them here, so the shape is not ours to flatten.
type Deploy struct {
	Resources Resources `yaml:"resources,omitempty"`
}

type Resources struct {
	Limits *Limits `yaml:"limits,omitempty"`
}

// Limits are compose's own spellings: CPUs is a decimal string ("0.5"), Memory a byte
// quantity with a unit suffix ("2048M").
type Limits struct {
	CPUs   string `yaml:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}

// Labels is compose's labels in either spelling: a mapping, or a list of "key=value".
type Labels map[string]string

func (labels *Labels) UnmarshalYAML(node *yaml.Node) error {
	out := Labels{}
	switch node.Kind {
	case yaml.MappingNode:
		if err := node.Decode((*map[string]string)(&out)); err != nil {
			return err
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			text, err := scalarText(item)
			if err != nil {
				return err
			}
			key, value, _ := strings.Cut(text, "=")
			out[key] = value
		}
	default:
		return fmt.Errorf("line %d: labels want a mapping or a list of key=value", node.Line)
	}
	*labels = out
	return nil
}

// StringList is a compose field that may be written as one scalar or as a sequence, which
// is true of nearly every list in the specification - `command: sh -c ...` and
// `command: ["sh", "-c", ...]` are both legal and both common in the wild. Numbers are
// accepted too, because `expose: [5432]` and `ports: [8080]` are how people write them.
// Marshalling always produces a sequence.
type StringList []string

func (list *StringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var value string
		if err := node.Decode(&value); err != nil {
			return err
		}
		*list = StringList{value}
		return nil
	case yaml.SequenceNode:
		out := make(StringList, 0, len(node.Content))
		for _, item := range node.Content {
			// The long syntax - `ports: [{target: 80, published: "8080"}]`,
			// `volumes: [{type: bind, source: /a, target: /b}]` - is how modern compose files
			// are written. Refusing it aborted the whole import over a spelling.
			if item.Kind == yaml.MappingNode {
				text, err := longFormEntry(item)
				if err != nil {
					return err
				}
				out = append(out, text)
				continue
			}
			text, err := scalarText(item)
			if err != nil {
				return err
			}
			out = append(out, text)
		}
		*list = out
		return nil
	default:
		return fmt.Errorf("line %d: want a string or a list of strings", node.Line)
	}
}

// longFormEntry flattens compose's long port/volume syntax into the short string form the
// rest of this package reads. Only the fields that survive the crossing into a Zinc app are
// looked at; the others (mode, consistency, bind options) do not reach the short form either
// and are dropped by the importer with a note, as they would have been anyway.
func longFormEntry(node *yaml.Node) (string, error) {
	fields := map[string]string{}
	for index := 0; index+1 < len(node.Content); index += 2 {
		value, err := scalarText(node.Content[index+1])
		if err != nil {
			continue // a nested mapping (e.g. volume driver options) has no short form
		}
		fields[strings.ToLower(node.Content[index].Value)] = value
	}
	switch {
	case fields["target"] != "" && fields["type"] != "": // a volume
		mount := fields["source"] + ":" + fields["target"]
		if fields["read_only"] == "true" {
			mount += ":ro"
		}
		return mount, nil
	case fields["target"] != "": // a port
		port := fields["target"]
		if published := fields["published"]; published != "" {
			port = published + ":" + port
		}
		if host := fields["host_ip"]; host != "" {
			port = host + ":" + port
		}
		if proto := fields["protocol"]; proto != "" {
			port += "/" + proto
		}
		return port, nil
	}
	return "", fmt.Errorf("line %d: a long-form entry needs at least a `target`", node.Line)
}

// scalarText reads one scalar as text, so a YAML integer (`expose: [5432]`) is the string
// a port list is made of rather than a decode error.
func scalarText(node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("line %d: want a string", node.Line)
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return "", err
	}
	switch typed := value.(type) {
	case string:
		return typed, nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(typed), nil
	default:
		return "", fmt.Errorf("line %d: want a string", node.Line)
	}
}

// Dependencies is compose's depends_on in either of its two spellings: the short list of
// names, and the long map from name to condition. Both are everywhere, so both decode;
// the long form is what gets written, because it is the one that can say "wait until
// healthy".
type Dependencies map[string]Depend

func (deps *Dependencies) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		out := Dependencies{}
		for _, item := range node.Content {
			name, err := scalarText(item)
			if err != nil {
				return err
			}
			out[name] = Depend{Condition: ConditionStarted}
		}
		*deps = out
		return nil
	case yaml.MappingNode:
		out := Dependencies{}
		if err := node.Decode((*map[string]Depend)(&out)); err != nil {
			return err
		}
		*deps = out
		return nil
	default:
		return fmt.Errorf("line %d: depends_on wants a list of names or a map of conditions", node.Line)
	}
}

// Names returns the dependency names in a stable order, which map iteration cannot give.
func (deps Dependencies) Names() []string {
	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

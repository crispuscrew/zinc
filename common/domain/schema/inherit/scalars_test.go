package inherit

import (
	"fmt"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResolveScalarSpellings(t *testing.T) {
	for _, scalar := range []string{"0022", "0x10", "1_024", "+001", "1e3", "1.2300", "TRUE", "2026-09-27", "!!str 0022"} {
		t.Run(scalar, func(t *testing.T) {
			base := fmt.Sprintf(`SchemaVersion: 4
StartConditions:
  EntrypointEnv: &environment {UMASK: &value %s, ALIAS: *value}
  AttachedEnv: *environment
CreatorFlags: &arguments [--mask, *value, %s]
RunnerFlags: *arguments
`, scalar, scalar)
			child := fmt.Sprintf(`Inherits: base
StartConditions:
  AttachedEnv: &environment {CHILD: &value %s, ALIAS: *value}
RunnerFlags: &arguments [--child, *value, %s]
`, scalar, scalar)
			baseConfig, childConfig := strictConfig(t, []byte(base)), strictConfig(t, []byte(child))
			config := resolveFrom(t, "leaf", map[string]string{
				"base": base, "child": child, "leaf": "Inherits: child\nHostTheme: false\n",
			})
			if !reflect.DeepEqual(config.StartConditions.EntrypointEnv, baseConfig.StartConditions.EntrypointEnv) ||
				!reflect.DeepEqual(config.StartConditions.AttachedEnv, childConfig.StartConditions.AttachedEnv) ||
				!reflect.DeepEqual(config.CreatorFlags, baseConfig.CreatorFlags) ||
				!reflect.DeepEqual(config.RunnerFlags, childConfig.RunnerFlags) {
				t.Fatalf("inheritance changed scalar spelling: env=%v attached=%v creator=%q runner=%q",
					config.StartConditions.EntrypointEnv, config.StartConditions.AttachedEnv, config.CreatorFlags, config.RunnerFlags)
			}
		})
	}
}

func TestMergePreservesScalarNodes(t *testing.T) {
	input := []byte("CreatorFlags: [&mask 0022, *mask, 0x10, 1e3, TRUE, !!str 0022, null]\n")
	var original yaml.Node
	if err := yaml.Unmarshal(input, &original); err != nil {
		t.Fatal(err)
	}
	output, err := Merge(input, []byte("AppNameID: child\n"))
	if err != nil {
		t.Fatal(err)
	}
	var merged yaml.Node
	if err := yaml.Unmarshal(output, &merged); err != nil {
		t.Fatalf("unreadable output: %v\n%s", err, output)
	}
	root := merged.Content[0]
	arguments := root.Content[findKey(root, "CreatorFlags")+1].Content
	for index, expected := range original.Content[0].Content[1].Content {
		if expected.Kind == yaml.AliasNode {
			expected = expected.Alias
		}
		actual := arguments[index]
		if actual.Kind != expected.Kind || actual.Tag != expected.Tag || actual.Value != expected.Value {
			t.Errorf("argument %d changed: kind/tag/value = %v/%q/%q, want %v/%q/%q", index,
				actual.Kind, actual.Tag, actual.Value, expected.Kind, expected.Tag, expected.Value)
		}
	}
}

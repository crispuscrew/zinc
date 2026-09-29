package inherit

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func expansionError(t *testing.T, input, expected string) {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(input), &document); err != nil {
		t.Fatalf("parse expansion fixture: %v", err)
	}
	if _, err := expandYAML(document.Content[0]); err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected %q expansion error, got %v", expected, err)
	}
}

func TestExpansionRejectsCyclesAndInvalidMerges(t *testing.T) {
	for _, testCase := range []struct{ input, expected string }{
		{"&cycle [*cycle]", "recursive YAML alias"},
		{"&cycle {<<: *cycle}", "recursive YAML alias"},
		{"{<<: 0022}", "requires a mapping"},
		{"{<<: [null]}", "requires a mapping"},
		{"{<<: [[{UMASK: 0022}]]}", "requires a mapping"},
		{"{UMASK: 0022, UMASK: 0077}", "duplicate YAML key"},
		{"{<<: {UMASK: 0022, UMASK: 0077}, UMASK: 0007}", "duplicate YAML key"},
		{"{<<: {}, <<: {}}", "duplicate YAML key"},
	} {
		expansionError(t, testCase.input, testCase.expected)
	}
}

func TestExpansionBoundsDepth(t *testing.T) {
	input := strings.Repeat("[", maxYAMLDepth+2) + "value" + strings.Repeat("]", maxYAMLDepth+2)
	expansionError(t, input, "depth limit")
}

func TestExpansionBoundsNodeAmplification(t *testing.T) {
	var input strings.Builder
	input.WriteString("level0: &level0 [value]\n")
	for level := 1; level < 20; level++ {
		fmt.Fprintf(&input, "level%d: &level%d [*level%d, *level%d]\n", level, level, level-1, level-1)
	}
	expansionError(t, input.String(), "size limit")
}

func TestExpansionBoundsScalarAmplification(t *testing.T) {
	value := strings.Repeat("value", 1024)
	input := "text: &text " + value + "\naliases: [" + strings.Repeat("*text, ", maxYAMLBytes/len(value)) + "]\n"
	expansionError(t, input, "size limit")
}

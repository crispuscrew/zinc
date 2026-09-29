package inherit

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

func TestResolveYAMLMergePrecedence(t *testing.T) {
	for _, testCase := range []struct{ before, after, mask string }{
		{mask: "0022"},
		{before: "    UMASK: 0007\n", mask: "0007"},
		{after: "    UMASK: 0007\n", mask: "0007"},
	} {
		config := resolveFrom(t, "child", map[string]string{
			"base": "StartConditions: {AttachedEnv: {OLD: inherited}}\nRunnerFlags: [base]\n",
			"child": fmt.Sprintf(`Inherits: base
StartConditions:
  EntrypointEnv: &defaults {UMASK: 0022, SOURCE: inherited}
  AttachedEnv:
%s    <<:
      - {<<: *defaults, SOURCE: first}
      - {UMASK: 0077, SOURCE: last, EXTRA: 0x10}
%s<<: {RunnerFlags: [0022, 0x10]}
`, testCase.before, testCase.after),
		})
		expected := map[string]string{"UMASK": testCase.mask, "SOURCE": "first", "EXTRA": "0x10"}
		if !reflect.DeepEqual(config.StartConditions.AttachedEnv, expected) ||
			!reflect.DeepEqual(config.RunnerFlags, []string{"0022", "0x10"}) {
			t.Fatalf("YAML merge precedence/spelling changed: %+v", config)
		}
	}
}

func TestMergeQuotedMergeKeyIsData(t *testing.T) {
	output, err := Merge([]byte(`StartConditions:
  EntrypointEnv: &environment {'<<': literal, UMASK: 0022}
  AttachedEnv: {<<: *environment}
`), []byte("AppNameID: child\n"))
	if err != nil {
		t.Fatal(err)
	}
	config := strictConfig(t, output)
	if config.StartConditions.AttachedEnv["<<"] != "literal" || config.StartConditions.AttachedEnv["UMASK"] != "0022" {
		t.Fatalf("quoted merge key was treated as syntax: %v", config.StartConditions.AttachedEnv)
	}
}

func TestMergeRejectsDuplicateSourceKeys(t *testing.T) {
	for _, mapping := range []string{
		"{UMASK: 0022, UMASK: 0077}",
		"{<<: {UMASK: 0022, UMASK: 0077}, UMASK: 0007}",
		"{<<: {UMASK: 0022}, <<: {UMASK: 0077}}",
	} {
		input := []byte("StartConditions: {EntrypointEnv: " + mapping + "}\n")
		for _, documents := range [][2][]byte{{input, []byte("AppNameID: child\n")}, {nil, input}} {
			if _, err := Merge(documents[0], documents[1]); err == nil {
				t.Fatalf("duplicate keys accepted: %s", mapping)
			}
		}
	}
}

func TestMergeKeepsUnknownFieldsForStrictDecode(t *testing.T) {
	output, err := Merge([]byte("StartConditions: {<<: &unknown {Unsupported: 0022}}\n"), []byte("AppNameID: child\n"))
	if err != nil {
		t.Fatal(err)
	}
	var config schema.AppConfig
	decoder := yaml.NewDecoder(bytes.NewReader(output))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err == nil || !strings.Contains(err.Error(), "Unsupported") {
		t.Fatalf("strict decoding lost the unknown merged field: %v\n%s", err, output)
	}
}

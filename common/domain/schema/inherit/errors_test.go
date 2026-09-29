package inherit

import (
	"fmt"
	"strings"
	"testing"
)

func TestResolveFailsClosed(t *testing.T) {
	for _, testCase := range []struct {
		files map[string]string
		want  string
	}{
		{map[string]string{"child": "Inherits: base\n", "base": "Inherits: child\n"}, "cycle"},
		{map[string]string{"child": "Inherits: ghost\n"}, "ghost"},
		{map[string]string{"child": "Inherits: ../../etc/evil\n"}, "Inherits"},
		{map[string]string{"child": "Inherits: base\n", "base": "Capabilities: [NET_RAW]\n"}, "base"},
		{map[string]string{"child": "Inherits: base\nCapabilities: []\n", "base": "Capabilities: [NET_RAW]\n"}, "Capabilities"},
	} {
		_, err := Resolve([]byte(testCase.files["child"]), loadFiles(testCase.files))
		if err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Fatalf("want %s error, got %v", testCase.want, err)
		}
	}
}

func TestResolveDepthBounded(t *testing.T) {
	files := map[string]string{}
	for index := 0; index <= maxDepth+2; index++ {
		files[fmt.Sprintf("app%d", index)] = fmt.Sprintf("Inherits: app%d\n", index+1)
	}
	_, err := Resolve([]byte(files["app0"]), loadFiles(files))
	if err == nil || !strings.Contains(err.Error(), "deeper than") {
		t.Fatalf("want depth error, got %v", err)
	}
}

func TestMergeRejectsNonMappingsAndDuplicates(t *testing.T) {
	for _, input := range []string{"- app\n", "scalar\n", "DBusMeta: {Talk: [], Talk: [org.example.App]}\n"} {
		if _, err := Merge([]byte(input), []byte("AppNameID: child\n")); err == nil {
			t.Fatalf("bad base accepted: %s", input)
		}
		if _, err := Merge([]byte("AppNameID: base\n"), []byte(input)); err == nil {
			t.Fatalf("bad child accepted: %s", input)
		}
	}
	if _, err := Merge(nil, []byte("AppNameID: child\n")); err != nil {
		t.Fatal(err)
	}
}

func TestParent(t *testing.T) {
	for _, testCase := range []struct {
		text, want string
		wantError  bool
	}{
		{"Inherits: base\n", "base", false},
		{"AppNameID: app\n", "", false},
		{"Inherits: '  '\n", "", false},
		{"Inherits: ../evil\n", "", true},
		{"Inherits: UPPER\n", "", true},
	} {
		parent, err := Parent([]byte(testCase.text))
		if (err != nil) != testCase.wantError || parent != testCase.want {
			t.Fatalf("Parent(%q) = %q, %v", testCase.text, parent, err)
		}
	}
}

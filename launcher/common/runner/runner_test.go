package runner

import (
	"os"
	"strings"
	"testing"
)

func TestActionsUseResolvedTypeAndStoreKey(t *testing.T) {
	for _, testCase := range []struct {
		appType string
		binary  string
		runArgs string
	}{
		{"ZincContainer", "zcr", "run\nchild\n--exec\n"},
		{"ZincVirtualization", "zvr", "run\nchild\n"},
	} {
		t.Run(testCase.appType, func(t *testing.T) {
			calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
			writeConfigs(t, map[string]string{
				"base":  "SchemaVersion: 4\nType: " + testCase.appType + "\nAppNameID: base\nRunnerFlags: ['--raw-backend-flag']\n",
				"child": "Inherits: base\nAppNameID: child\nLauncherMeta:\n  Description: another-app\n  Group: category\n  Icon: icon-name\n",
			})
			if err := Launch("child"); err != nil {
				t.Fatal(err)
			}
			if err := Stop("child"); err != nil {
				t.Fatal(err)
			}
			want := testCase.binary + "\n" + testCase.runArgs + testCase.binary + "\nstop\nchild\n"
			assertCalls(t, calls, want)
		})
	}
}

func TestActionsRejectInvalidConfigsBeforeExecution(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
		want string
	}{
		{"invalid-type", "AppNameID: child\nType: Other\n", "unsupported Type"},
		{"missing-type", "AppNameID: child\n", "unsupported Type"},
		{"wrong-identity", "Inherits: base\n", "resolves to AppNameID"},
		{"unknown-field", "AppNameID: child\nBogus: true\n", "field Bogus"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
			writeConfigs(t, map[string]string{
				"base": "SchemaVersion: 4\nType: ZincVirtualization\nAppNameID: base\n", "child": testCase.body,
			})
			for _, action := range []func(string) error{Launch, Stop} {
				if err := action("child"); err == nil || !strings.Contains(err.Error(), testCase.want) {
					t.Fatalf("want %q, got %v", testCase.want, err)
				}
			}
			if _, err := os.Stat(calls); !os.IsNotExist(err) {
				t.Fatalf("runtime executed for an invalid config: %v", err)
			}
		})
	}
}

func TestActionsRejectUnsafeNamesBeforeLookup(t *testing.T) {
	fakeRuntimes(t, nil)
	for _, name := range []string{"--net=host", "-x", "", "notes.yaml", "app.yaml"} {
		for _, action := range []func(string) error{Launch, Stop} {
			if err := action(name); err == nil || strings.Contains(err.Error(), "not found") {
				t.Errorf("action(%q): want name guard error, got %v", name, err)
			}
		}
	}
}

func TestActionsReportSelectedRuntimeErrors(t *testing.T) {
	for _, appType := range []string{"ZincContainer", "ZincVirtualization"} {
		t.Run(appType, func(t *testing.T) {
			fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
			writeConfigs(t, map[string]string{"bad": "SchemaVersion: 4\nAppNameID: bad\nType: " + appType + "\n"})
			if err := Launch("bad"); err == nil || !strings.Contains(err.Error(), "bad: no such app") {
				t.Fatalf("want runtime error, got %v", err)
			}
		})
	}
}

package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchPreservesExplicitPathAndResolvesStoreParent(t *testing.T) {
	for _, testCase := range []struct {
		appType string
		binary  string
		extra   string
	}{
		{"ZincContainer", "zcr", "--exec\n"},
		{"ZincVirtualization", "zvr", ""},
	} {
		t.Run(testCase.appType, func(t *testing.T) {
			calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
			writeConfigs(t, map[string]string{"base": "SchemaVersion: 4\nType: " + testCase.appType + "\nAppNameID: base\n"})
			path := filepath.Join(t.TempDir(), "explicit path.yaml")
			if err := os.WriteFile(path, []byte("Inherits: base\nAppNameID: distinct-name\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := Launch(path); err != nil {
				t.Fatal(err)
			}
			assertCalls(t, calls, testCase.binary+"\nrun\n"+path+"\n"+testCase.extra)
		})
	}
}

func TestLaunchMissingSelectedRuntimeDoesNotFallBack(t *testing.T) {
	for _, testCase := range []struct {
		appType string
		missing string
		present string
	}{
		{"ZincContainer", "zcr", "zvr"},
		{"ZincVirtualization", "zvr", "zcr"},
	} {
		t.Run(testCase.appType, func(t *testing.T) {
			calls := fakeRuntimes(t, map[string]string{testCase.present: fakeScript})
			writeConfigs(t, map[string]string{"child": "AppNameID: child\nType: " + testCase.appType + "\n"})
			if err := Launch("child"); err == nil || !strings.Contains(err.Error(), testCase.missing+" not found on $PATH") {
				t.Fatalf("want selected runtime not found, got %v", err)
			}
			if _, err := os.Stat(calls); !os.IsNotExist(err) {
				t.Fatalf("unexpected fallback runtime call: %v", err)
			}
		})
	}
}

func TestLaunchMissingConfigDoesNotExecute(t *testing.T) {
	calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
	writeConfigs(t, nil)
	if err := Launch("missing"); err == nil || !strings.Contains(err.Error(), "store: read missing") {
		t.Fatalf("want missing config error, got %v", err)
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatalf("runtime executed without a config: %v", err)
	}
}

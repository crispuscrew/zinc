package runner

import (
	"os"
	"strings"
	"testing"
)

func TestContainerInstancesResolveBaseAndPreserveAddress(t *testing.T) {
	for _, testCase := range []struct{ base, address string }{
		{"shell", "shell@work"},
		{"shell", "shell@work-laptop"},
		{"shell", "shell@w0rk_2"},
		{"shell.local", "shell.local@work"},
		{"shell.yaml", "shell.yaml@work"},
	} {
		t.Run(testCase.address, func(t *testing.T) {
			calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
			writeConfigs(t, map[string]string{
				"base":           "SchemaVersion: 4\nType: ZincContainer\nAppNameID: base\n",
				testCase.base:    "Inherits: base\nAppNameID: " + testCase.base + "\n",
				testCase.address: "Type: ZincVirtualization\nAppNameID: " + testCase.address + "\n",
			})
			if err := Launch(testCase.address); err != nil {
				t.Fatal(err)
			}
			if err := Stop(testCase.address); err != nil {
				t.Fatal(err)
			}
			assertCalls(t, calls, "zcr\nrun\n"+testCase.address+"\n--exec\nzcr\nstop\n"+testCase.address+"\n")
		})
	}
}

func TestInstanceActionsRejectInvalidAddresses(t *testing.T) {
	for _, testCase := range []struct{ address, want string }{
		{"shell@", "instance"},
		{"@work", "instance"},
		{"shell@Work", "instance"},
		{"shell@work laptop", "instance"},
		{"shell@work.local", "instance"},
		{"shell@-work", "instance"},
		{"shell@work@home", "instance"},
		{" shell@work", "instance"},
		{"shell@work ", "instance"},
		{"shell@work\n", "instance"},
		{"shell@..", "instance"},
		{"..@work", "instance"},
		{"-shell@work", "cannot begin"},
		{"shell@work.yaml", "cannot end"},
	} {
		t.Run(testCase.address, func(t *testing.T) {
			calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
			writeConfigs(t, map[string]string{"shell": "Type: ZincContainer\nAppNameID: shell\n"})
			for _, action := range []func(string) error{Launch, Stop} {
				if err := action(testCase.address); err == nil || !strings.Contains(err.Error(), testCase.want) || strings.Contains(err.Error(), "store: read") {
					t.Errorf("action(%q): want address guard %q, got %v", testCase.address, testCase.want, err)
				}
			}
			if _, err := os.Stat(calls); !os.IsNotExist(err) {
				t.Fatalf("runtime executed for invalid address: %v", err)
			}
		})
	}
}

func TestVMInstancesAreExplicitlyUnsupported(t *testing.T) {
	calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
	writeConfigs(t, map[string]string{
		"base":       "SchemaVersion: 4\nType: ZincVirtualization\nAppNameID: base\n",
		"guest":      "Inherits: base\nAppNameID: guest\n",
		"guest@work": "Type: ZincContainer\nAppNameID: guest@work\n",
	})
	for _, action := range []func(string) error{Launch, Stop} {
		if err := action("guest@work"); err == nil || !strings.Contains(err.Error(), "VM instances are unsupported") {
			t.Errorf("want explicit VM instance rejection, got %v", err)
		}
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatalf("runtime executed for VM instance: %v", err)
	}
}

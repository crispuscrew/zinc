package runner

import (
	"reflect"
	"strings"
	"testing"
)

func TestRunningCombinesRuntimeFormats(t *testing.T) {
	calls := fakeRuntimes(t, map[string]string{"zcr": fakeScript, "zvr": fakeScript})
	running, err := Running()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"firefox": true, "syncthing": true, "guest": true}
	if !reflect.DeepEqual(running, want) {
		t.Fatalf("Running = %v, want %v", running, want)
	}
	assertCalls(t, calls, "zcr\nps\nzvr\nps\n")
}

func TestRunningUsesAvailableRuntime(t *testing.T) {
	for _, binary := range []string{"zcr", "zvr"} {
		t.Run(binary, func(t *testing.T) {
			fakeRuntimes(t, map[string]string{binary: fakeScript})
			running, err := Running()
			if err != nil {
				t.Fatal(err)
			}
			name := "firefox"
			if binary == "zvr" {
				name = "guest"
			}
			if !running[name] {
				t.Fatalf("available %s app missing from %v", binary, running)
			}
		})
	}
}

func TestRunningBothUnavailable(t *testing.T) {
	fakeRuntimes(t, nil)
	_, err := Running()
	if err == nil || !strings.Contains(err.Error(), "zcr not found") || !strings.Contains(err.Error(), "zvr not found") {
		t.Fatalf("want both runtime errors, got %v", err)
	}
}

func TestRunningEmptyVMListIsNotAnApp(t *testing.T) {
	fakeRuntimes(t, map[string]string{"zvr": "#!/bin/sh\nprintf 'no guests running\\n'\n"})
	running, err := Running()
	if err != nil || len(running) != 0 {
		t.Fatalf("Running = %v, %v; want empty set", running, err)
	}
}

package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureForwardsSuccessfulRuntimeWarnings(t *testing.T) {
	for _, binary := range []string{"zcr", "zvr"} {
		t.Run(binary, func(t *testing.T) {
			warning := "warning: RunnerFlags bypass Zinc policy checks\n"
			script := "#!/bin/sh\nprintf 'started child\\n'\nprintf '" + warning + "' >&2\n"
			fakeRuntimes(t, map[string]string{binary: script})
			stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
			if err != nil {
				t.Fatal(err)
			}
			original := os.Stderr
			os.Stderr = stderr
			t.Cleanup(func() {
				os.Stderr = original
				if err := stderr.Close(); err != nil {
					t.Error(err)
				}
			})
			output, err := capture(binary, "run", "child")
			if err != nil || output != "started child\n" {
				t.Fatalf("capture = %q, %v", output, err)
			}
			data, err := os.ReadFile(stderr.Name())
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != warning {
				t.Fatalf("user stderr = %q, want %q", data, warning)
			}
		})
	}
}

func TestCapturePreservesFailureDiagnostics(t *testing.T) {
	for _, binary := range []string{"zcr", "zvr"} {
		t.Run(binary, func(t *testing.T) {
			fakeRuntimes(t, map[string]string{binary: "#!/bin/sh\nprintf 'warning: RunnerFlags\\nlaunch failed\\n' >&2\nexit 1\n"})
			_, err := capture(binary, "run", "child")
			if err == nil || !strings.Contains(err.Error(), "warning: RunnerFlags\nlaunch failed") {
				t.Fatalf("want warnings and failure in UI error, got %v", err)
			}
		})
	}
}

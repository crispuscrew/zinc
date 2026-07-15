package main

import (
	"os"
	"strings"
	"testing"
)

const digestPin = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// quiet redirects stdout to /dev/null for the duration of the test, so the commands'
// success prints don't clutter test output.
func quiet(t *testing.T) {
	t.Helper()
	old := os.Stdout
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = null
	t.Cleanup(func() { os.Stdout = old; null.Close() })
}

func TestRunUsageAndUnknown(t *testing.T) {
	if err := run(nil); err == nil || !strings.Contains(err.Error(), "usage: zcc") {
		t.Fatalf("no args should return usage, got: %v", err)
	}
	if err := run([]string{"bogus"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("a bogus command should be rejected, got: %v", err)
	}
}

func TestVersionDispatch(t *testing.T) {
	quiet(t)
	if err := run([]string{"version"}); err != nil {
		t.Fatalf("version: %v", err)
	}
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("--version: %v", err)
	}
}

// The authoring commands (new/list/validate/delete) work locally against the store, with
// no runtime needed. XDG_CONFIG_HOME isolates the store to a temp dir.
func TestAuthoringLifecycle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	if err := run([]string{"new", "demo", "--image", "docker.io/library/alpine" + digestPin}); err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := run([]string{"list"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := run([]string{"validate", "demo"}); err != nil {
		t.Fatalf("validate: %v", err)
	}
	// A duplicate name is refused.
	if err := run([]string{"new", "demo", "--image", "docker.io/library/alpine" + digestPin}); err == nil {
		t.Fatal("new should refuse an existing name")
	}
	if err := run([]string{"delete", "demo"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := run([]string{"validate", "demo"}); err == nil {
		t.Fatal("validate should fail after the app is deleted")
	}
}

// new with a non-digest-pinned third-party image is rejected by validation (§5.5).
func TestNewRejectsUnpinnedImage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)
	if err := run([]string{"new", "demo", "--image", "alpine:latest"}); err == nil {
		t.Fatal("new should reject a non-digest-pinned third-party image")
	}
}

// The runtime commands delegate to zcr; with none on $PATH they fail with the delegate's
// actionable error rather than doing anything locally.
func TestRuntimeDelegateNeedsZcr(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir → no zcr
	for _, cmd := range []string{"run", "build", "stop", "logs", "image"} {
		err := run([]string{cmd, "demo"})
		if err == nil || !strings.Contains(err.Error(), "not found on $PATH") {
			t.Fatalf("%s with no zcr: want the delegate not-found error, got: %v", cmd, err)
		}
	}
}

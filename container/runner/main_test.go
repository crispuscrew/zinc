package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const digestPin = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// quiet redirects stdout to /dev/null for the duration of the test, so the dry-run plan
// and validate output don't clutter test output.
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

// writeApp writes an app file to a temp dir and returns its path (an argument with a
// path separator is read directly, no store lookup).
func writeApp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunUsageAndUnknown(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := run(nil); err == nil || !strings.Contains(err.Error(), "usage: zcr") {
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

func TestValidateDispatch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	good := writeApp(t, "SchemaVersion: 2\nType: ZincContainer\nAppNameID: demo\nImageMeta:\n  Image: docker.io/library/alpine"+digestPin+"\n")
	if err := run([]string{"validate", good}); err != nil {
		t.Fatalf("validate of a good app should pass, got: %v", err)
	}

	bad := writeApp(t, "SchemaVersion: 2\nType: ZincContainer\nAppNameID: demo\nImageMeta:\n  Image: alpine:latest\n")
	if err := run([]string{"validate", bad}); err == nil {
		t.Fatal("validate of a non-digest-pinned image should fail")
	}

	if err := run([]string{"validate"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("validate with no arg should return usage, got: %v", err)
	}
}

// run without --exec is a dry run: it validates and prints the podman plan, touching no
// runtime, so it succeeds for a valid app under test.
func TestRunDryRun(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	good := writeApp(t, "SchemaVersion: 2\nType: ZincContainer\nAppNameID: demo\nImageMeta:\n  Image: docker.io/library/alpine"+digestPin+"\n")
	if err := run([]string{"run", good}); err != nil {
		t.Fatalf("dry-run of a good app should succeed, got: %v", err)
	}
}

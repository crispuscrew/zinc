package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mainFakeZcr = `#!/bin/sh
case "$1" in
  run) if [ "$2" = "bad" ]; then echo "bad: nope" 1>&2; exit 1; fi; exit 0 ;;
  ps) exit 0 ;;
  *) exit 2 ;;
esac
`

func fakeZcr(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zcr"), []byte(mainFakeZcr), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRun_Version(t *testing.T) {
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("--version: %v", err)
	}
}

func TestRun_Usage(t *testing.T) {
	if err := run([]string{"-h"}); err != nil {
		t.Fatalf("-h: %v", err)
	}
}

func TestRun_TooManyArgs(t *testing.T) {
	err := run([]string{"one", "two"})
	if err == nil || !strings.Contains(err.Error(), "too many arguments") {
		t.Fatalf("want a too-many-arguments error, got %v", err)
	}
}

func TestRun_DirectLaunch(t *testing.T) {
	fakeZcr(t)
	if err := run([]string{"firefox"}); err != nil {
		t.Fatalf("zlt firefox: %v", err)
	}
}

func TestRun_DirectLaunchSurfacesError(t *testing.T) {
	fakeZcr(t)
	err := run([]string{"bad"})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("want the zcr error surfaced, got %v", err)
	}
}

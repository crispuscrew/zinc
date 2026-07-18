package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeScript stands in for the zcr binary: `ps` prints two running apps, `run bad`
// fails with a stderr message, everything else succeeds.
const fakeScript = `#!/bin/sh
case "$1" in
  ps) printf 'firefox\nsyncthing\n' ;;
  run) if [ "$2" = "bad" ]; then echo "bad: no such app" 1>&2; exit 1; fi; exit 0 ;;
  stop) exit 0 ;;
  *) echo "unknown: $*" 1>&2; exit 2 ;;
esac
`

// fakeZcr installs a fake zcr on $PATH for the duration of the test.
func fakeZcr(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zcr"), []byte(fakeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestLaunch_OK(t *testing.T) {
	fakeZcr(t)
	if err := Launch("firefox"); err != nil {
		t.Fatalf("Launch(firefox): %v", err)
	}
}

func TestLaunch_SurfacesError(t *testing.T) {
	fakeZcr(t)
	err := Launch("bad")
	if err == nil || !strings.Contains(err.Error(), "no such app") {
		t.Fatalf("Launch(bad): want the zcr error surfaced, got %v", err)
	}
}

func TestStop_OK(t *testing.T) {
	fakeZcr(t)
	if err := Stop("firefox"); err != nil {
		t.Fatalf("Stop(firefox): %v", err)
	}
}

func TestRunning(t *testing.T) {
	fakeZcr(t)
	running, err := Running()
	if err != nil {
		t.Fatal(err)
	}
	if !running["firefox"] || !running["syncthing"] || len(running) != 2 {
		t.Fatalf("Running = %v, want {firefox, syncthing}", running)
	}
}

// With no zcr on $PATH, actions fail with an actionable message.
func TestLaunch_ZcrNotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // an empty dir: no zcr
	err := Launch("firefox")
	if err == nil || !strings.Contains(err.Error(), "not found on $PATH") {
		t.Fatalf("want a not-found error, got %v", err)
	}
}

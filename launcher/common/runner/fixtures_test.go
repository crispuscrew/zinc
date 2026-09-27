package runner

import (
	"os"
	"path/filepath"
	"testing"
)

const fakeScript = `#!/bin/sh
printf '%s\n' "${0##*/}" "$@" >> "$RUNNER_CALLS"
case "$1" in
  ps) case "${0##*/}" in
    zcr) printf 'firefox\nsyncthing\n' ;;
    zvr) printf 'APP                      PID      STATE\nguest                    123      running\n' ;;
    esac ;;
  run) if [ "$2" = "bad" ]; then printf 'bad: no such app\n' >&2; exit 1; fi ;;
  stop) exit 0 ;;
  *) exit 2 ;;
esac
`

func fakeRuntimes(t *testing.T, scripts map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	for binary, script := range scripts {
		if err := os.WriteFile(filepath.Join(directory, binary), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	calls := filepath.Join(directory, "calls")
	t.Setenv("RUNNER_CALLS", calls)
	return calls
}

func writeConfigs(t *testing.T, configs map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)
	appsDirectory := filepath.Join(directory, "zinc", "apps")
	if err := os.MkdirAll(appsDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range configs {
		if err := os.WriteFile(filepath.Join(appsDirectory, name+".yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return appsDirectory
}

func assertCalls(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("runtime calls = %q, want %q", data, want)
	}
}

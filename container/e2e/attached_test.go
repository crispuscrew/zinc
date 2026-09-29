//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (env environment) attached(t *testing.T) {
	// The emulator shim removes only the TTY allocation, retaining the real exec argv.
	directory := t.TempDir()
	emulator := filepath.Join(directory, "terminal")
	script := "#!/bin/sh\n[ \"$1\" = podman ] && [ \"$2\" = exec ] && [ \"$3\" = -it ] || exit 91\nshift 3\nexec podman exec \"$@\"\n"
	if err := os.WriteFile(emulator, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZINC_TERMINAL", emulator)
	body := fmt.Sprintf(`SchemaVersion: 4
Type: ZincContainer
AppNameID: attached
ImageMeta: {Image: %s}
DisplayMeta: {DisableGpuAccess: true}
StartConditions:
  Terminal: true
  Attached: true
  Entrypoint: echo fallback
  EntrypointEnv: {BASE: keep, VALUE: base}
  AttachedEntrypoint: printf '%%s|%%s|%%s\n' "$BASE" "$VALUE" "$EXTRA" >> /sessions/events
  AttachedEnv: {VALUE: first, EXTRA: session}
StopConditions: {Background: true}
Volumes:
  - HostMounted: true
    HostMount: %s
    InnerMount: /sessions
    Writable: true
`, appImage, directory)
	path := filepath.Join(env.apps, "attached.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	defer tool(env.runner, "stop", "attached")
	env.start(t, "attached")
	waitEvent := func(want string) {
		t.Helper()
		if !waitFor(func() bool {
			data, _ := os.ReadFile(filepath.Join(directory, "events"))
			return strings.Contains(string(data), want)
		}) {
			t.Fatalf("session did not write %q", want)
		}
	}
	waitEvent("keep|first|session")
	identity := must(t, "podman", "inspect", "--format", "{{.Id}}", "attached")
	body = strings.Replace(body, "VALUE: first", "VALUE: second", 1)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	must(t, env.runner, "run", "attached", "--exec")
	waitEvent("keep|second|session")
	if got := must(t, "podman", "inspect", "--format", "{{.Id}}", "attached"); got != identity {
		t.Fatal("reopen replaced the live holder")
	}
	must(t, env.runner, "term", "attached")
	if !waitFor(func() bool {
		data, _ := os.ReadFile(filepath.Join(directory, "events"))
		return strings.Count(string(data), "keep|second|session") == 2
	}) {
		t.Fatal("term did not propagate the latest session environment")
	}
}

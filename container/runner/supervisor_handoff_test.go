package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func supervisorFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv(supervisorTestRoot, root)
	t.Setenv("XDG_RUNTIME_DIR", root)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
case "$1 $2" in
  "container exists") test -f "$ZINC_TEST_SUPERVISOR_ROOT/resource" && test ! -f "$ZINC_TEST_SUPERVISOR_ROOT/exited" ;;
  "wait client")
    : > "$ZINC_TEST_SUPERVISOR_ROOT/waiting"
    while test ! -f "$ZINC_TEST_SUPERVISOR_ROOT/exited"; do sleep 0.01; done ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(filepath.Join(root, "podman"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile(filepath.Join(root, "exited"), nil, 0o600) })
	path := filepath.Join(root, "explicit.yaml")
	body := "SchemaVersion: 4\nType: ZincContainer\nAppNameID: client\nImageMeta: {Image: localhost/client:local}\n" +
		"LauncherMeta: {Description: original snapshot}\nStartConditions:\n  EntrypointEnv: {SESSION: original}\n" +
		"NetworkMeta:\n  Interfaces: [{ID: uplink}]\n  RulesByPriority:\n    - From: {Type: Self, Interface: uplink}\n      To: {Type: Internet}\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func awaitSupervisorFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			return data
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("supervisor did not produce %s", path)
	return nil
}

func TestSupervisorFileLaunchKeepsResolvedSnapshot(t *testing.T) {
	root, path := supervisorFixture(t)
	svc := supervisorService(root, false)
	want, err := loadLaunchable(svc, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmdRun(svc, options.HostOptions{}, []string{path, "--exec"}); err != nil {
		t.Fatal(err)
	}
	awaitSupervisorFile(t, filepath.Join(root, "waiting"))
	if _, err := os.Stat(filepath.Join(root, "cleaned.json")); !os.IsNotExist(err) {
		t.Fatal("cleaned up a live app")
	}
	// A store entry created/edited after launch must not replace the inherited snapshot.
	if err := os.Mkdir(filepath.Join(root, "apps"), 0o700); err != nil {
		t.Fatal(err)
	}
	shadow := filepath.Join(root, "apps", "client.yaml")
	if err := os.WriteFile(shadow, []byte("SchemaVersion: 4\nAppNameID: changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken: after launch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "exited"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var got schema.AppConfig
	if err := json.Unmarshal(awaitSupervisorFile(t, filepath.Join(root, "cleaned.json")), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup reloaded changed config:\ngot %+v\nwant %+v", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "resource")); !os.IsNotExist(err) {
		t.Fatal("prepared resources leaked")
	}
	// The same filepath can launch again after cleanup, without a registered app.
	if err := os.Remove(shadow); err != nil {
		t.Fatal(err)
	}
	data, err := svc.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"exited", "waiting", "cleaned.json"} {
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmdRun(svc, options.HostOptions{}, []string{path, "--exec"}); err != nil {
		t.Fatal(err)
	}
	awaitSupervisorFile(t, filepath.Join(root, "waiting"))
	if err := os.WriteFile(filepath.Join(root, "exited"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	awaitSupervisorFile(t, filepath.Join(root, "cleaned.json"))
}

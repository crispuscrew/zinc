package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/hyprzinc/core/domain"
)

// digestPin is a canonical sha256 digest (64 hex chars) — the form §5.5 requires for
// third-party images, so saved/marshalled fixtures pass Validate.
const digestPin = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func sampleApp(name string) domain.AppConfig {
	cfg, _ := domain.DefaultsFor(domain.PresetStrict)
	cfg.App.Name = name
	cfg.App.Image = "docker.io/library/" + name + digestPin
	return cfg
}

func tempStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Root: t.TempDir()}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	sto := tempStore(t)
	want := sampleApp("firefox")
	if err := sto.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := sto.Load("firefox")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.App.Name != want.App.Name || got.App.Image != want.App.Image ||
		got.Network.Mode != want.Network.Mode || got.Display.Wayland != want.Display.Wayland {
		t.Fatalf("round-trip mismatch:\n got: %+v\nwant: %+v", got, want)
	}
	if err := domain.Validate(got); err != nil {
		t.Fatalf("round-tripped config does not validate: %v", err)
	}
}

func TestListExistsDelete(t *testing.T) {
	sto := tempStore(t)

	if names, err := sto.List(); err != nil || len(names) != 0 {
		t.Fatalf("empty store: names=%v err=%v", names, err)
	}
	if sto.Exists("firefox") {
		t.Fatal("Exists should be false before save")
	}

	for _, name := range []string{"zed", "firefox", "ghostty"} {
		if err := sto.Save(sampleApp(name)); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}

	names, err := sto.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := names; len(got) != 3 || got[0] != "firefox" || got[1] != "ghostty" || got[2] != "zed" {
		t.Fatalf("List not sorted/complete: %v", got)
	}
	if !sto.Exists("firefox") {
		t.Fatal("Exists should be true after save")
	}

	if err := sto.Delete("firefox"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if sto.Exists("firefox") {
		t.Fatal("Exists should be false after delete")
	}
	if err := sto.Delete("firefox"); err != nil {
		t.Fatalf("deleting a missing app should be a no-op, got: %v", err)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	sto := tempStore(t)
	bad := sampleApp("firefox")
	bad.App.Image = "alpine:latest" // third-party, not digest-pinned (§5.5)

	if err := sto.Save(bad); err == nil {
		t.Fatal("Save should reject invalid config")
	}
	if sto.Exists("firefox") {
		t.Fatal("nothing should be written when validation fails")
	}
}

func TestMarshalLoadRoundtrip(t *testing.T) {
	// The $EDITOR flow marshals a draft, lets the user edit it, then re-reads via
	// Load — which rejects unknown keys. So Marshal's output must round-trip cleanly.
	cfg, _ := domain.DefaultsFor(domain.PresetNetworked)
	cfg.App.Name = "rt"
	cfg.App.Image = "docker.io/x" + digestPin
	cfg.Network.IPv4CIDR = []string{"1.1.1.1/32"}
	cfg.Network.Ports = []int{443}

	data, err := Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "rt.toml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("round-trip Load failed (Marshal emitted a key Load rejects?): %v", err)
	}
	if got.App.Image != cfg.App.Image || got.Network.Mode != cfg.Network.Mode ||
		len(got.Network.IPv4CIDR) != 1 || len(got.Network.Ports) != 1 {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, cfg)
	}
}

func TestLoad_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	const body = `schema_version = 1
[app]
name = "x"
image = "img@sha256:abc"
typpo = "drift"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected unknown-key error, got: %v", err)
	}
}

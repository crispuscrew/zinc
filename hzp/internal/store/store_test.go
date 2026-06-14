package store

import (
	"testing"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
)

func sampleApp(name string) config.AppConfig {
	cfg, _ := config.DefaultsFor(config.PresetStrict)
	cfg.App.Name = name
	cfg.App.Image = "docker.io/library/" + name + "@sha256:abc"
	return cfg
}

func tempStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Root: t.TempDir()}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := tempStore(t)
	want := sampleApp("firefox")
	if err := s.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.Load("firefox")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.App.Name != want.App.Name || got.App.Image != want.App.Image ||
		got.Network.Mode != want.Network.Mode || got.Display.Wayland != want.Display.Wayland {
		t.Fatalf("round-trip mismatch:\n got: %+v\nwant: %+v", got, want)
	}
	// What we save back must itself validate (no drift introduced by encoding).
	if err := config.Validate(got); err != nil {
		t.Fatalf("round-tripped config does not validate: %v", err)
	}
}

func TestListExistsDelete(t *testing.T) {
	s := tempStore(t)

	// Missing directory → empty, not an error.
	if names, err := s.List(); err != nil || len(names) != 0 {
		t.Fatalf("empty store: names=%v err=%v", names, err)
	}
	if s.Exists("firefox") {
		t.Fatal("Exists should be false before save")
	}

	for _, n := range []string{"zed", "firefox", "ghostty"} {
		if err := s.Save(sampleApp(n)); err != nil {
			t.Fatalf("save %s: %v", n, err)
		}
	}

	names, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := names; len(got) != 3 || got[0] != "firefox" || got[1] != "ghostty" || got[2] != "zed" {
		t.Fatalf("List not sorted/complete: %v", got)
	}
	if !s.Exists("firefox") {
		t.Fatal("Exists should be true after save")
	}

	if err := s.Delete("firefox"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if s.Exists("firefox") {
		t.Fatal("Exists should be false after delete")
	}
	if err := s.Delete("firefox"); err != nil {
		t.Fatalf("deleting a missing app should be a no-op, got: %v", err)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	s := tempStore(t)
	bad := sampleApp("firefox")
	bad.App.Image = "alpine:latest" // third-party, not digest-pinned (§5.5)

	if err := s.Save(bad); err == nil {
		t.Fatal("Save should reject invalid config")
	}
	if s.Exists("firefox") {
		t.Fatal("nothing should be written when validation fails")
	}
}

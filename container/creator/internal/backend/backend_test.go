package backend

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/creator/internal/store"
)

// validApp is a minimal config that passes validate.Validate (a localhost image is
// exempt from the digest-pin rule), so Save accepts it.
func validApp(name string) schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     name,
		ImageMeta:     schema.ImageMeta{Image: "localhost/app:local"},
	}
}

// newBackend builds a backend over a temp-dir store seeded with the named apps. The
// runtime side (svc.Running, via zcr) is not on $PATH in tests, so Rename's running
// check is skipped - which is the intended behaviour for pure authoring.
func newBackend(t *testing.T, names ...string) Service {
	t.Helper()
	svc := New(&store.Store{Root: t.TempDir()})
	for _, name := range names {
		if err := svc.Save(validApp(name)); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}
	return svc
}

// Renaming onto an existing app must be refused, not silently overwrite the target and
// delete the source (which would destroy the target's definition without confirmation).
func TestRename_RefusesExistingTarget(t *testing.T) {
	svc := newBackend(t, "alpha", "beta")
	if err := svc.Rename("alpha", "beta"); err == nil {
		t.Fatal("rename onto an existing app should be refused")
	}
	if !svc.Exists("alpha") || !svc.Exists("beta") {
		t.Fatal("a refused rename must leave both apps intact")
	}
}

// A rename to a fresh name moves the definition: old gone, new present.
func TestRename_ToFreshNameMovesIt(t *testing.T) {
	svc := newBackend(t, "alpha")
	if err := svc.Rename("alpha", "gamma"); err != nil {
		t.Fatalf("rename to a fresh name: %v", err)
	}
	if svc.Exists("alpha") {
		t.Fatal("the old name should be gone after rename")
	}
	if !svc.Exists("gamma") {
		t.Fatal("the new name should exist after rename")
	}
}

// Empty and unchanged targets are rejected up front, before any file is touched.
func TestRename_RejectsEmptyAndUnchanged(t *testing.T) {
	svc := newBackend(t, "alpha")
	if err := svc.Rename("alpha", ""); err == nil {
		t.Fatal("an empty target should be rejected")
	}
	if err := svc.Rename("alpha", "alpha"); err == nil {
		t.Fatal("an unchanged target should be rejected")
	}
	if !svc.Exists("alpha") {
		t.Fatal("alpha must still exist after the rejected renames")
	}
}

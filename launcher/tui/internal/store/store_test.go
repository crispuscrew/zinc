package store

import (
	"os"
	"path/filepath"
	"testing"
)

func writeApp(t *testing.T, dir, name, desc string) {
	t.Helper()
	body := "SchemaVersion: 2\nType: ZincContainer\nAppNameID: " + name +
		"\nDescription: " + desc + "\nImageMeta:\n  Image: localhost/app:local\n"
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// List returns app names (without the .yaml suffix), sorted, ignoring non-yaml files.
func TestList(t *testing.T) {
	dir := t.TempDir()
	writeApp(t, dir, "firefox", "browser")
	writeApp(t, dir, "alacritty", "terminal")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}
	sto := &Store{Root: dir}

	names, err := sto.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "alacritty" || names[1] != "firefox" {
		t.Fatalf("List = %v, want [alacritty firefox]", names)
	}
}

// A missing store directory lists as empty, not an error.
func TestList_MissingDirIsEmpty(t *testing.T) {
	sto := &Store{Root: filepath.Join(t.TempDir(), "does-not-exist")}
	names, err := sto.List()
	if err != nil {
		t.Fatalf("missing dir should not error, got %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("missing dir should be empty, got %v", names)
	}
}

// Load decodes an app file and its display fields.
func TestLoad(t *testing.T) {
	dir := t.TempDir()
	writeApp(t, dir, "firefox", "Web browser")
	sto := &Store{Root: dir}

	cfg, err := sto.Load("firefox")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppNameID != "firefox" || cfg.Description != "Web browser" {
		t.Fatalf("Load = %+v, want AppNameID=firefox Description='Web browser'", cfg)
	}
}

// A crafted name (path separator or ".." segment) cannot read a file outside the store.
func TestLoad_RejectsUnsafeNames(t *testing.T) {
	sto := &Store{Root: t.TempDir()}
	for _, bad := range []string{"../evil", "sub/app", "..", ""} {
		if _, err := sto.Load(bad); err == nil {
			t.Errorf("Load(%q): want error, got nil", bad)
		}
	}
}

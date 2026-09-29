package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreRejectsUnsafeNames(t *testing.T) {
	sto := tempStore(t)
	for _, name := range []string{"../evil", "sub/app", "..", ".", ""} {
		if _, err := sto.Load(name); err == nil {
			t.Errorf("Load(%q) accepted", name)
		}
		if err := sto.Delete(name); err == nil {
			t.Errorf("Delete(%q) accepted", name)
		}
		if sto.Exists(name) {
			t.Errorf("Exists(%q) accepted", name)
		}
	}
	if err := safeName("my..app"); err != nil {
		t.Fatal(err)
	}
}

func TestListScreensNamesButKeepsDottedKeys(t *testing.T) {
	sto := tempStore(t)
	for _, name := range []string{"notes.yaml", "notes.yaml.yaml", "--net=host.yaml", "Firefox.yaml", "has space.yaml", ".hidden.yaml"} {
		if err := os.WriteFile(filepath.Join(sto.Root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	names, err := sto.List()
	if err != nil || !reflect.DeepEqual(names, []string{"notes", "notes.yaml"}) {
		t.Fatal(names, err)
	}
}

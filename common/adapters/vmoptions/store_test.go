package vmoptions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	domain "github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func fixture() domain.Config {
	config := domain.Default("guest", "/images/base.qcow2")
	config.BaseDigest = "sha256:" + strings.Repeat("a", 64)
	return config
}

func TestLoadDoesNotCreateDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	if _, err := Load(root, "guest"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("load created directories")
	}
}

func TestSaveRoundTripPermissionsAndConflict(t *testing.T) {
	root := t.TempDir()
	config := fixture()
	if err := Save(root, config); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, config); err != nil {
		t.Fatal("identical save must be idempotent:", err)
	}
	loaded, err := Load(root, "guest")
	if err != nil || loaded.BaseDigest != config.BaseDigest {
		t.Fatalf("%+v %v", loaded, err)
	}
	info, err := os.Stat(File(root, "guest"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode: %v %v", info, err)
	}
	for _, path := range []string{"zinc", "zinc/runtime", "zinc/runtime/vm"} {
		info, err := os.Stat(filepath.Join(root, path))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("directory %s mode", path)
		}
	}
	config.BaseDigest = "sha256:" + strings.Repeat("b", 64)
	if err := Save(root, config); err == nil {
		t.Fatal("overwrote different existing content")
	}
	unchanged, err := Load(root, "guest")
	if err != nil || unchanged.BaseDigest != loaded.BaseDigest {
		t.Fatal("conflicting save changed file")
	}
}

func TestConcurrentSavesHaveOneWinner(t *testing.T) {
	root := t.TempDir()
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, digit := range []string{"a", "b"} {
		group.Add(1)
		go func(digit string) {
			defer group.Done()
			config := fixture()
			config.BaseDigest = "sha256:" + strings.Repeat(digit, 64)
			results <- Save(root, config)
		}(digit)
	}
	group.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d", winners)
	}
	if _, err := Load(root, "guest"); err != nil {
		t.Fatal(err)
	}
}

func TestStrictBoundedJSON(t *testing.T) {
	valid, err := json.Marshal(fixture())
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"unknown":          strings.Replace(string(valid), `"Version":1`, `"Version":1,"Extra":true`, 1),
		"duplicate":        strings.Replace(string(valid), `"Version":1`, `"Version":1,"Version":1`, 1),
		"case alias":       strings.Replace(string(valid), `"Version":1`, `"version":1`, 1),
		"trailing":         string(valid) + " {}",
		"null scalar":      strings.Replace(string(valid), `"Version":1`, `"Version":null`, 1),
		"nested duplicate": strings.Replace(string(valid), `"ForwardPorts":null`, `"ForwardPorts":[{"HostPort":2222,"HostPort":2223,"GuestPort":22}]`, 1),
		"oversized":        strings.Repeat(" ", maxBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "options.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadFile(path); err == nil {
				t.Fatal("accepted invalid JSON")
			}
		})
	}
}

func TestTraversalBindingAndSymlinkRefused(t *testing.T) {
	root := t.TempDir()
	if File(root, "../other") != "" {
		t.Fatal("unsafe name produced a path")
	}
	if err := Save(root, fixture()); err != nil {
		t.Fatal(err)
	}
	path := File(root, "guest")
	other := filepath.Join(root, "other.json")
	if err := os.Rename(path, other); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "guest"); err == nil {
		t.Fatal("followed symlink")
	}
	if err := Save(root, fixture()); err == nil {
		t.Fatal("saved over symlink")
	}
}

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crispuscrew/zinc/menu"
)

func writeConfigs(t *testing.T, configs map[string]string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)
	appsDirectory := filepath.Join(directory, "zinc", "apps")
	if err := os.MkdirAll(appsDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range configs {
		if err := os.WriteFile(filepath.Join(appsDirectory, name+".yaml"), []byte("SchemaVersion: 4\n"+body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadItemsResolvesNestedMetadataAndPreservesKeys(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	writeConfigs(t, map[string]string{
		"base":  "Type: ZincVirtualization\nAppNameID: base\nLauncherMeta:\n  Description: inherited description\n  Group: Tools\n  Icon: base-icon\n",
		"child": "Inherits: base\nAppNameID: child\nLauncherMeta:\n  Description: display-only\n",
		"other": "Type: ZincContainer\nAppNameID: other\n",
		"wrong": "Inherits: base\n",
	})
	items, err := loadItems()
	if err != nil {
		t.Fatal(err)
	}
	want := []menu.Item{
		{Label: "base", Description: "inherited description", Group: "Tools", Icon: "base-icon"},
		{Label: "child", Description: "display-only", Group: "Tools", Icon: "base-icon"},
		{Label: "other"},
		{Label: "wrong"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %+v, want %+v", items, want)
	}
}

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crispuscrew/zinc/launcher/tui/internal/tui"
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

func TestLoadAppsResolvesNestedMetadataAndPreservesKeys(t *testing.T) {
	writeConfigs(t, map[string]string{
		"base":  "Type: ZincVirtualization\nAppNameID: base\nLauncherMeta:\n  Description: inherited description\n  Group: Tools\n  Icon: base-icon\n",
		"child": "Inherits: base\nAppNameID: child\n",
		"wrong": "Inherits: base\n",
	})
	apps, err := loadApps()
	if err != nil {
		t.Fatal(err)
	}
	want := []tui.App{
		{Name: "base", Description: "inherited description"},
		{Name: "child", Description: "inherited description"},
		{Name: "wrong"},
	}
	if !reflect.DeepEqual(apps, want) {
		t.Fatalf("apps = %+v, want %+v", apps, want)
	}
}

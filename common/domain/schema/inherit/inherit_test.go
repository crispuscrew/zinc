package inherit

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

func loadFiles(files map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		text, exists := files[name]
		if !exists {
			return nil, fmt.Errorf("no app %q defined", name)
		}
		return []byte(text), nil
	}
}

func resolveFrom(t *testing.T, app string, files map[string]string) schema.AppConfig {
	t.Helper()
	data, err := Resolve([]byte(files[app]), loadFiles(files))
	if err != nil {
		t.Fatalf("Resolve(%s): %v", app, err)
	}
	return strictConfig(t, data)
}

func strictConfig(t *testing.T, data []byte) schema.AppConfig {
	t.Helper()
	var config schema.AppConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		t.Fatalf("strict decode: %v\n%s", err, data)
	}
	return config
}

func TestResolveChildOverridesBase(t *testing.T) {
	config := resolveFrom(t, "child", map[string]string{
		"base": `SchemaVersion: 3
Type: ZincContainer
AppNameID: base
ImageMeta: {Image: 'localhost/base:local'}
ResourcesMeta: {MaxRamMiB: 256, PIDsLimit: 64}
HostTheme: true
DisplayMeta: {DisableSecurityContext: true}
DBusMeta: {Talk: [org.example.One, org.example.Two], Own: [org.example.App]}
`,
		"child": `SchemaVersion: 3
AppNameID: child
Inherits: base
ResourcesMeta: {MaxRamMiB: 1024}
HostTheme: false
DisplayMeta: {DisableSecurityContext: false}
DBusMeta: {Talk: []}
`,
	})
	if config.AppNameID != "child" || config.SchemaVersion != schema.SchemaVersion ||
		config.ImageMeta.Image != "localhost/base:local" || config.ResourcesMeta.MaxRamMiB != 1024 ||
		config.ResourcesMeta.PIDsLimit != 64 || config.HostTheme || config.DisplayMeta.DisableSecurityContext ||
		len(config.DBusMeta.Talk) != 0 || len(config.DBusMeta.Own) != 1 {
		t.Fatalf("explicit child overrides or omitted base values lost: %+v", config)
	}
}

func TestResolveStatedListReplaces(t *testing.T) {
	config := resolveFrom(t, "child", map[string]string{
		"base": `SchemaVersion: 4
DBusMeta: {Talk: [org.example.One, org.example.Two]}
NetworkMeta:
  RulesByPriority:
    - From: {Type: Self}
      To: {Type: Internet, Filter: {Ports: [443]}}
      Protocols: [TCP]
`,
		"child": "Inherits: base\nDBusMeta: {Talk: [org.example.Child]}\n",
	})
	if len(config.DBusMeta.Talk) != 1 || config.DBusMeta.Talk[0] != "org.example.Child" ||
		len(config.NetworkMeta.RulesByPriority) != 1 || config.NetworkMeta.RulesByPriority[0].To.Filter.Ports[0] != 443 {
		t.Fatalf("list replacement or inherited rules changed: %+v", config)
	}
}

func TestResolveChain(t *testing.T) {
	config := resolveFrom(t, "leaf", map[string]string{
		"root":   "SchemaVersion: 4\nImageMeta: {Image: root}\nResourcesMeta: {MaxRamMiB: 128, PIDsLimit: 16}\n",
		"middle": "Inherits: root\nResourcesMeta: {MaxRamMiB: 512}\n",
		"leaf":   "AppNameID: leaf\nInherits: middle\nIcon: firefox\n",
	})
	if config.AppNameID != "leaf" || config.ImageMeta.Image != "root" || config.ResourcesMeta.MaxRamMiB != 512 ||
		config.ResourcesMeta.PIDsLimit != 16 || config.LauncherMeta.Icon != "firefox" {
		t.Fatalf("ancestor-first migration/merge failed: %+v", config)
	}
}

func TestResolveNoInheritanceIsUntouched(t *testing.T) {
	const text = "# comment\nSchemaVersion: 4\nAppNameID: solo\nImageMeta: {Image: solo}\n"
	output, err := Resolve([]byte(text), func(string) ([]byte, error) {
		t.Fatal("no parent should be loaded")
		return nil, nil
	})
	if err != nil || string(output) != text {
		t.Fatalf("solo config changed: %v\n%s", err, output)
	}
}

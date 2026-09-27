package store

import (
	"os"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func writeApp(t *testing.T, sto *Store, name, text string) {
	t.Helper()
	if err := os.MkdirAll(sto.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sto.Path(name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadResolvedMergesWithoutFlattening(t *testing.T) {
	sto := tempStore(t)
	writeApp(t, sto, "base", "SchemaVersion: 3\nType: ZincContainer\nAppNameID: base\nImageMeta:\n  Image: localhost/base:local\nResourcesMeta:\n  MaxRamMiB: 256\n  PIDsLimit: 64\nHostTheme: true\nRunnerFlags: [--label=parent]\n")
	writeApp(t, sto, "child", "SchemaVersion: 3\nType: ZincContainer\nAppNameID: child\nInherits: base\nResourcesMeta:\n  MaxRamMiB: 1024\nHostTheme: false\nRunnerFlags: []\n")
	raw, err := sto.Load("child")
	if err != nil {
		t.Fatal(err)
	}
	if raw.ImageMeta.Image != "" || raw.SchemaVersion != schema.SchemaVersion {
		t.Fatal("raw read flattened or failed migration", raw)
	}
	resolved, err := sto.LoadResolved("child")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ImageMeta.Image != "localhost/base:local" || resolved.ResourcesMeta.MaxRamMiB != 1024 || resolved.ResourcesMeta.PIDsLimit != 64 || resolved.HostTheme || len(resolved.RunnerFlags) != 0 {
		t.Fatal(resolved)
	}
}

func TestSaveRefusesInheritedDataLoss(t *testing.T) {
	sto := tempStore(t)
	const document = "SchemaVersion: 4\nType: ZincContainer\nAppNameID: child\nInherits: base\nLauncherMeta:\n  Icon: firefox\n"
	writeApp(t, sto, "child", document)
	cfg, err := sto.Load("child")
	if err != nil {
		t.Fatal(err)
	}
	if err := sto.Save(cfg); err == nil || !strings.Contains(err.Error(), "inherits from") {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(sto.Path("child"))
	if err != nil || string(actual) != document {
		t.Fatal("refused save modified sparse YAML")
	}
	if err := sto.Save(sampleApp("solo")); err != nil {
		t.Fatal(err)
	}
}

func TestLoadResolvedFailsClosed(t *testing.T) {
	sto := tempStore(t)
	writeApp(t, sto, "orphan", "SchemaVersion: 4\nAppNameID: orphan\nInherits: ghost\n")
	writeApp(t, sto, "loop-a", "SchemaVersion: 4\nAppNameID: loop-a\nInherits: loop-b\n")
	writeApp(t, sto, "loop-b", "SchemaVersion: 4\nAppNameID: loop-b\nInherits: loop-a\n")
	writeApp(t, sto, "escape", "SchemaVersion: 4\nAppNameID: escape\nInherits: ../../etc/evil\n")
	for name, wanted := range map[string]string{"orphan": "ghost", "loop-a": "cycle", "escape": "Inherits"} {
		if _, err := sto.LoadResolved(name); err == nil || !strings.Contains(err.Error(), wanted) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestResolvedAppMustKeepOwnIdentity(t *testing.T) {
	sto := tempStore(t)
	writeApp(t, sto, "browser", "SchemaVersion: 4\nType: ZincContainer\nAppNameID: browser\nImageMeta:\n  Image: localhost/browser:local\n")
	writeApp(t, sto, "notes", "SchemaVersion: 4\nInherits: browser\nLauncherMeta:\n  Icon: notes\n")
	if _, err := sto.LoadResolved("notes"); err == nil || !strings.Contains(err.Error(), "keep its own name") {
		t.Fatal(err)
	}
	writeApp(t, sto, "notes", "SchemaVersion: 4\nAppNameID: notes\nInherits: browser\nLauncherMeta:\n  Icon: notes\n")
	cfg, err := sto.LoadResolved("notes")
	if err != nil || cfg.ImageMeta.Image != "localhost/browser:local" {
		t.Fatal(cfg, err)
	}
}

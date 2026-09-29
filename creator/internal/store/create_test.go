package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	vmfiles "github.com/crispuscrew/zinc/common/adapters/vmoptions"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func vmDefinition() (schema.AppConfig, vmoptions.Config) {
	cfg := schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincVirtualization, AppNameID: "guest", ImageMeta: schema.ImageMeta{Image: "/images/base.qcow2"}, ResourcesMeta: schema.ResourcesMeta{MaxCPUCores: 2, MaxRamMiB: 4096}}
	options := vmoptions.Default(cfg.AppNameID, cfg.ImageMeta.Image)
	options.BaseDigest = "sha256:" + strings.Repeat("a", 64)
	return cfg, options
}

func TestCreateNeverReplacesExistingAppOrVMOptions(t *testing.T) {
	for _, collision := range []string{"app", "options"} {
		t.Run(collision, func(t *testing.T) {
			sto := tempStore(t)
			cfg, options := vmDefinition()
			path := sto.Path(cfg.AppNameID)
			if collision == "options" {
				path = sto.VMPath(cfg.AppNameID)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("preserve authored bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := sto.Create(cfg, &options); err == nil {
				t.Fatal("existing definition was replaced")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "preserve authored bytes" {
				t.Fatalf("existing bytes lost: %s %v", data, err)
			}
			other := sto.VMPath(cfg.AppNameID)
			if collision == "options" {
				other = sto.Path(cfg.AppNameID)
			}
			if _, err := os.Stat(other); !os.IsNotExist(err) {
				t.Fatalf("partial definition left at %s: %v", other, err)
			}
		})
	}
}

func TestCreateValidatesBothBeforePublication(t *testing.T) {
	sto := tempStore(t)
	cfg, options := vmDefinition()
	options.Image = "/images/different.qcow2"
	if err := sto.Create(cfg, &options); err == nil {
		t.Fatal("different image binding accepted")
	}
	if sto.Exists(cfg.AppNameID) {
		t.Fatal("partial YAML published")
	}
}

func TestSaveVMNoOpDoesNotRewriteOptions(t *testing.T) {
	sto := tempStore(t)
	cfg, options := vmDefinition()
	if err := sto.Create(cfg, &options); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(sto.VMPath(cfg.AppNameID))
	if err != nil {
		t.Fatal(err)
	}
	cfg.LauncherMeta.Description = "changed shared metadata"
	if err := sto.SaveDefinition(cfg, &options, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(sto.VMPath(cfg.AppNameID))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("shared-only edit rewrote runtime options")
	}
	loaded, err := vmfiles.Load(sto.ConfigRoot(), cfg.AppNameID)
	if err != nil || loaded.BaseDigest != options.BaseDigest {
		t.Fatalf("runtime options lost: %+v %v", loaded, err)
	}
}

func TestVMEditUpdatesBothImageBindings(t *testing.T) {
	sto := tempStore(t)
	cfg, options := vmDefinition()
	if err := sto.Create(cfg, &options); err != nil {
		t.Fatal(err)
	}
	cfg.ImageMeta.Image, options.Image = "/images/new.qcow2", "/images/new.qcow2"
	options.BaseDigest = "sha256:" + strings.Repeat("b", 64)
	if err := sto.SaveDefinition(cfg, &options, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := sto.Load(cfg.AppNameID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sto.LoadVM(loaded); err != nil {
		t.Fatal(err)
	}
}

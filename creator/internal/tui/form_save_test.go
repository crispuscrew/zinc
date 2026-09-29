package tui

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/keys"
	"github.com/crispuscrew/zinc/creator/internal/store"
)

func TestFormDetectsConcurrentEdits(t *testing.T) {
	model, storage := newLoaded(t, "app")
	model = send(model, key("e"))
	changed, err := storage.Load("app")
	if err != nil {
		t.Fatal(err)
	}
	changed.LauncherMeta.Description = "another author's work"
	if err := storage.Save(changed); err != nil {
		t.Fatal(err)
	}
	model.form.desc.SetValue("stale form")
	model = send(model, key("ctrl+s"))
	if model.mode != modeForm || model.form.err == nil || !strings.Contains(model.form.err.Error(), "changed while editing") {
		t.Fatal("stale form overwrote another author")
	}
	actual, err := storage.Load("app")
	if err != nil || actual.LauncherMeta.Description != changed.LauncherMeta.Description {
		t.Fatal(actual, err)
	}
}

func TestVMFormCreatesBothDocuments(t *testing.T) {
	storage := &store.Store{Root: t.TempDir()}
	model := New(backend.New(storage), keys.Active{})
	model.openForm(schema.AppConfig{Type: schema.ZincVirtualization}, true)
	model.form.name.SetValue("guest")
	model.form.image.SetValue("/images/base.qcow2")
	model.form.baseDigest.SetValue("sha256:" + strings.Repeat("a", 64))
	model = send(model, key("ctrl+s"))
	if model.mode != modeList {
		t.Fatalf("VM form did not save: %v", model.form.err)
	}
	cfg, err := storage.Load("guest")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.LoadVM(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestVMEditorRoundTripKeepsTypedOptions(t *testing.T) {
	model := New(backend.New(&store.Store{Root: t.TempDir()}), keys.Active{})
	model.openForm(schema.AppConfig{Type: schema.ZincVirtualization}, true)
	model.form.name.SetValue("guest")
	model.form.image.SetValue("/images/base.qcow2")
	model.form.baseDigest.SetValue("sha256:" + strings.Repeat("a", 64))
	model.form.diskSize.SetValue("42")
	model.form.vmMedia.SetValue(`["/images/tools.iso"]`)
	model.form.focus(fieldIdx(model.form, "advanced"))
	updated, _ := model.Update(key("enter"))
	model = updated.(Model)
	cfg := model.form.toConfig()
	cfg.ImageMeta.PublicSSHKeyPath = "/keys/new.pub"
	model = send(model, editedMsg{cfg: cfg})
	options, err := model.form.options(model.form.toConfig())
	if err != nil {
		t.Fatal(err)
	}
	if options.DiskSizeGiB != 42 || len(options.InstallMedia) != 1 || model.form.ciKey.Value() != "/keys/new.pub" {
		t.Fatal("editor lost options or key")
	}
}

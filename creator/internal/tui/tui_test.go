package tui

import (
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/keys"
	"github.com/crispuscrew/zinc/creator/internal/store"
	"testing"
)

func TestListNavigationAndQuit(t *testing.T) {
	model, _ := newLoaded(t, "alpha", "beta", "gamma")
	if len(model.apps) != 3 {
		t.Fatal(model.apps)
	}
	for range 3 {
		model = send(model, key("j"))
	}
	if model.cursor != 2 {
		t.Fatal("cursor escaped bottom")
	}
	model = send(model, key("k"))
	if model.cursor != 1 {
		t.Fatal("up failed")
	}
	model = send(model, key("q"))
	if !model.quitting {
		t.Fatal("quit failed")
	}
}

func TestListToFormModes(t *testing.T) {
	model, _ := newLoaded(t, "alpha")
	created := send(model, key("n"))
	if created.mode != modeForm || created.form == nil || !created.form.creating {
		t.Fatal("new form missing")
	}
	edited := send(model, key("e"))
	if edited.mode != modeForm || edited.form.creating || edited.form.draft.AppNameID != "alpha" {
		t.Fatal("wrong edit form")
	}
	back := send(edited, key("esc"))
	if back.mode != modeList || back.form != nil {
		t.Fatal("cancel did not discard form")
	}
}

func TestDeleteAndRenameFlows(t *testing.T) {
	model, storage := newLoaded(t, "alpha", "beta")
	model = send(model, key("R"))
	if model.mode != modeRename || model.renameFrom != "alpha" {
		t.Fatal("wrong rename target")
	}
	model.rename.SetValue("gamma")
	updated, command := model.Update(key("enter"))
	model = updated.(Model)
	if model.mode != modeList || command == nil {
		t.Fatal("rename did not schedule")
	}
	command()
	if storage.Exists("alpha") || !storage.Exists("gamma") || !storage.Exists("beta") {
		t.Fatal("rename affected wrong files")
	}
	model = send(model, loadApps(model.svc)())
	model = send(model, key("d"))
	if model.mode != modeConfirmDelete || model.confirmName != "beta" {
		t.Fatal("wrong deletion target")
	}
	updated, command = model.Update(key("y"))
	if updated.(Model).mode != modeList || command == nil {
		t.Fatal("delete did not schedule")
	}
	command()
	if storage.Exists("beta") || !storage.Exists("gamma") {
		t.Fatal("delete affected wrong files")
	}
}

func TestRenameUnchangedIsNoOp(t *testing.T) {
	model, storage := newLoaded(t, "alpha")
	model = send(model, key("R"))
	updated, command := model.Update(key("enter"))
	if command != nil || updated.(Model).mode != modeList || !storage.Exists("alpha") {
		t.Fatal("unchanged rename was destructive")
	}
}

func TestSaveValidAndInvalid(t *testing.T) {
	for _, valid := range []bool{false, true} {
		storage := &store.Store{Root: t.TempDir()}
		model := New(backend.New(storage), keys.Active{})
		model.openForm(schema.AppConfig{}, true)
		model.form.name.SetValue("app")
		model.form.image.SetValue("alpine:latest")
		if valid {
			model.form.image.SetValue(img("app"))
		}
		model = send(model, key("ctrl+s"))
		if valid && (model.mode != modeList || !storage.Exists("app")) {
			t.Fatal("valid save failed")
		}
		if !valid && (model.mode != modeForm || model.form.err == nil || storage.Exists("app")) {
			t.Fatal("invalid save wrote data or lost error")
		}
	}
}

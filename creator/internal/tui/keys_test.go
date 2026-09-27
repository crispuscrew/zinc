package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/keys"
	"strings"
	"testing"
)

func TestSchemeDrivesKeys(t *testing.T) {
	defaultModel, _ := newLoaded(t, "alpha", "beta")
	if _, command := defaultModel.Update(key("g")); command == nil {
		t.Fatal("default refresh missing")
	}
	vim, _ := loadedWith(t, keys.Active{Name: "vim", Scheme: keys.Vim}, "alpha", "beta")
	if _, command := vim.Update(key("g")); command != nil {
		t.Fatal("vim inherited default refresh binding")
	}
	vim = send(vim, key("j"))
	if vim.cursor != 1 {
		t.Fatal("vim movement failed")
	}
	form := newForm(schema.AppConfig{}, true)
	start := form.idx
	form.update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if form.idx != start {
		t.Fatal("default scheme accepted vim form navigation")
	}
	form.scheme = keys.Vim
	form.update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if form.idx == start {
		t.Fatal("vim form navigation missing")
	}
}

func TestKeysPicker(t *testing.T) {
	model, _ := newLoaded(t, "alpha")
	updated, command := model.Update(key("?"))
	model = updated.(Model)
	if model.mode != modeKeys || command == nil {
		t.Fatal("picker did not open")
	}
	model = send(model, schemesMsg{names: []string{"default", "vim"}})
	model = send(model, key("j"))
	if model.keysCursor != 1 {
		t.Fatal("picker movement failed")
	}
	model = send(model, key("esc"))
	if model.mode != modeList {
		t.Fatal("picker did not close")
	}
}

func TestShellAndBuildActionsAreConditional(t *testing.T) {
	model, storage := newLoaded(t, "alpha", "beta")
	cfg := sample("alpha")
	cfg.StartConditions = schema.StartConditions{Terminal: true, Attached: true, Entrypoint: "sh"}
	cfg.ImageMeta.Install = []string{"apk add htop"}
	if err := storage.Save(cfg); err != nil {
		t.Fatal(err)
	}
	model = send(model, loadApps(backend.New(storage))())
	for _, binding := range []string{"S", "b"} {
		if _, command := model.Update(key(binding)); command == nil {
			t.Errorf("missing action %s", binding)
		}
	}
	model = send(model, key("j"))
	for binding, hint := range map[string]string{"S": "Attached", "b": "nothing to build"} {
		updated, command := model.Update(key(binding))
		if command != nil || !strings.Contains(updated.(Model).status, hint) {
			t.Errorf("unexpected %s action", binding)
		}
	}
}

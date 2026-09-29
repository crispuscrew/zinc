package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"testing"
)

func TestFormTypedValuesAndInstall(t *testing.T) {
	form := newForm(schema.AppConfig{}, true)
	form.name.SetValue("hollywood")
	form.image.SetValue(img("debian"))
	form.entrypoint.SetValue(" hollywood --speed 2 ")
	form.install.SetValue(" apt-get update \n apt-get install -y hollywood \n\n")
	cfg := form.toConfig()
	if cfg.AppNameID != "hollywood" || cfg.ImageMeta.Image != img("debian") || cfg.StartConditions.Entrypoint != "hollywood --speed 2" {
		t.Fatal(cfg)
	}
	if len(cfg.ImageMeta.Install) != 2 || cfg.ImageMeta.Install[0] != "apt-get update" {
		t.Fatal(cfg.ImageMeta.Install)
	}
	if err := validate.Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestFormClearToggleResolveAndAdvanced(t *testing.T) {
	form := newForm(schema.AppConfig{}, true)
	form.focus(fieldIdx(form, "image"))
	form.image.SetValue(img("app"))
	if _, result := form.update(key("ctrl+d")); result != formStay || form.image.Value() != "" {
		t.Fatal("clear failed")
	}
	if _, result := form.update(tea.KeyMsg{Type: tea.KeyCtrlR}); result != formResolve {
		t.Fatal("resolve missing on image")
	}
	form.focus(fieldIdx(form, "description"))
	if _, result := form.update(tea.KeyMsg{Type: tea.KeyCtrlR}); result != formStay {
		t.Fatal("resolved a description")
	}
	form.focus(fieldIdx(form, "terminal"))
	form.update(key("enter"))
	if !form.draft.StartConditions.Terminal {
		t.Fatal("terminal toggle failed")
	}
	form.focus(fieldIdx(form, "advanced"))
	if _, result := form.update(key("enter")); result != formEdit {
		t.Fatal("advanced editor missing")
	}
}

func TestEditorMessagesAndImageResolution(t *testing.T) {
	model, _ := newLoaded(t, "alpha")
	model = send(model, key("e"))
	model = send(model, editedMsg{err: errTest})
	if model.mode != modeForm || model.form.err == nil {
		t.Fatal("editor error disappeared")
	}
	edited := sample("alpha")
	edited.ImageMeta.Image = img("new")
	model = send(model, editedMsg{cfg: edited})
	if model.form.image.Value() != edited.ImageMeta.Image {
		t.Fatal("editor reload failed")
	}
	model = send(model, resolvedMsg{ref: img("pinned")})
	if model.form.image.Value() != img("pinned") {
		t.Fatal("resolved image lost")
	}
	created := newForm(schema.AppConfig{}, true)
	created.reload(sample("new"))
	if !created.creating {
		t.Fatal("reload reset creating state")
	}
}

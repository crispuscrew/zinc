package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/keys"
	"github.com/crispuscrew/zinc/creator/internal/store"
	"strings"
	"testing"
)

var errTest = errors.New("bad YAML")

func key(spec string) tea.KeyMsg {
	kinds := map[string]tea.KeyType{"tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab, "enter": tea.KeyEnter, "esc": tea.KeyEsc, "ctrl+s": tea.KeyCtrlS, "ctrl+d": tea.KeyCtrlD, "left": tea.KeyLeft, "right": tea.KeyRight}
	if kind, ok := kinds[spec]; ok {
		return tea.KeyMsg{Type: kind}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(spec)}
}

func img(name string) string {
	return "docker.io/library/" + name + "@sha256:" + strings.Repeat("a", 64)
}
func sample(name string) schema.AppConfig {
	return schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer, AppNameID: name, ImageMeta: schema.ImageMeta{Image: img(name)}}
}

func newLoaded(t *testing.T, names ...string) (Model, *store.Store) {
	t.Helper()
	return loadedWith(t, keys.Active{}, names...)
}

func loadedWith(t *testing.T, active keys.Active, names ...string) (Model, *store.Store) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	storage := &store.Store{Root: t.TempDir()}
	for _, name := range names {
		if err := storage.Save(sample(name)); err != nil {
			t.Fatal(err)
		}
	}
	service := backend.New(storage)
	model := New(service, active)
	updated, _ := model.Update(loadApps(service)())
	return updated.(Model), storage
}

func send(model Model, message tea.Msg) Model {
	updated, _ := model.Update(message)
	return updated.(Model)
}
func fieldIdx(form *formModel, label string) int {
	for index, field := range form.fields {
		if field.label == label {
			return index
		}
	}
	return -1
}

package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
	"github.com/crispuscrew/hyprzinc/hzp/internal/runspec"
	"github.com/crispuscrew/hyprzinc/hzp/internal/store"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func sample(name string) config.AppConfig {
	c, _ := config.DefaultsFor(config.PresetStrict)
	c.App.Name = name
	c.App.Image = "docker.io/library/" + name + "@sha256:abc"
	return c
}

// newLoaded returns a model whose store holds the given apps, already loaded.
func newLoaded(t *testing.T, names ...string) (Model, *store.Store) {
	t.Helper()
	st := &store.Store{Root: t.TempDir()}
	for _, n := range names {
		if err := st.Save(sample(n)); err != nil {
			t.Fatalf("seed %s: %v", n, err)
		}
	}
	m := New(st, runspec.Options{})
	tm, _ := m.Update(loadApps(st)()) // run the load command synchronously
	return tm.(Model), st
}

func send(m Model, msg tea.Msg) Model {
	tm, _ := m.Update(msg)
	return tm.(Model)
}

func TestListNavigation(t *testing.T) {
	m, _ := newLoaded(t, "alpha", "beta", "gamma") // List() sorts → alpha,beta,gamma
	if len(m.apps) != 3 {
		t.Fatalf("want 3 apps, got %d", len(m.apps))
	}
	m = send(m, key("j"))
	m = send(m, key("j"))
	if m.cursor != 2 {
		t.Fatalf("cursor after 2×down = %d, want 2", m.cursor)
	}
	m = send(m, key("j")) // clamp at bottom
	if m.cursor != 2 {
		t.Fatalf("cursor should clamp at 2, got %d", m.cursor)
	}
	m = send(m, key("k"))
	if m.cursor != 1 {
		t.Fatalf("cursor after up = %d, want 1", m.cursor)
	}
}

func TestListToFormModes(t *testing.T) {
	m, _ := newLoaded(t, "alpha")

	create := send(m, key("n"))
	if create.mode != modeForm || create.form == nil || !create.form.creating {
		t.Fatalf("n should open a create form")
	}

	edit := send(m, key("e"))
	if edit.mode != modeForm || edit.form == nil || edit.form.creating {
		t.Fatalf("e should open an edit form for the selected app")
	}
	if edit.form.draft.App.Name != "alpha" {
		t.Fatalf("edit form should load 'alpha', got %q", edit.form.draft.App.Name)
	}

	// esc returns to the list without saving.
	back := send(edit, key("esc"))
	if back.mode != modeList || back.form != nil {
		t.Fatalf("esc should return to the list")
	}
}

func TestQuit(t *testing.T) {
	m, _ := newLoaded(t)
	q := send(m, key("q"))
	if !q.quitting {
		t.Fatal("q should set quitting")
	}
}

func TestDeleteFlow(t *testing.T) {
	m, st := newLoaded(t, "alpha", "beta")
	m = send(m, key("d")) // confirm delete of 'alpha'
	if m.mode != modeConfirmDelete || m.confirmName != "alpha" {
		t.Fatalf("d should ask to confirm deleting alpha, got mode=%v name=%q", m.mode, m.confirmName)
	}
	// 'y' returns a remove command; run it.
	tm, cmd := m.Update(key("y"))
	m = tm.(Model)
	if m.mode != modeList {
		t.Fatalf("after confirm, mode should be list")
	}
	if cmd == nil {
		t.Fatal("confirm should return a remove command")
	}
	cmd() // perform the delete
	if st.Exists("alpha") {
		t.Fatal("alpha should be deleted from the store")
	}
	if !st.Exists("beta") {
		t.Fatal("beta should be untouched")
	}
}

func TestSaveInvalidStaysInForm(t *testing.T) {
	st := &store.Store{Root: t.TempDir()}
	m := New(st, runspec.Options{})
	m.mode = modeForm
	m.form = newForm(config.AppConfig{}, true)
	m.form.name.SetValue("x")
	m.form.image.SetValue("alpine:latest") // third-party, not digest-pinned (§5.5)

	m = send(m, key("ctrl+s"))
	if m.mode != modeForm {
		t.Fatal("invalid save should keep the form open")
	}
	if m.form == nil || m.form.err == nil {
		t.Fatal("invalid save should surface an error in the form")
	}
	if st.Exists("x") {
		t.Fatal("nothing should be written when validation fails")
	}
}

func TestSaveValid(t *testing.T) {
	st := &store.Store{Root: t.TempDir()}
	m := New(st, runspec.Options{})
	m.mode = modeForm
	m.form = newForm(config.AppConfig{}, true)
	m.form.name.SetValue("zed")
	m.form.image.SetValue("docker.io/zed@sha256:abc")

	m = send(m, key("ctrl+s"))
	if m.mode != modeList {
		t.Fatalf("valid save should return to the list, got mode %v", m.mode)
	}
	if !st.Exists("zed") {
		t.Fatal("valid save should persist the app")
	}
}

func TestFormPresetReseed(t *testing.T) {
	f := newForm(config.AppConfig{}, true)
	if f.draft.Network.Mode != config.NetworkNone || f.draft.Display.Wayland != config.WaylandSecurityContext {
		t.Fatalf("create form should start from strict defaults: %+v", f.draft)
	}
	f.applyPreset(config.PresetNetworked)
	if f.draft.Network.Mode != config.NetworkPasta || f.draft.Display.Wayland != config.WaylandPassthrough {
		t.Fatalf("networked preset should reseed mode/wayland: %+v", f.draft)
	}
	if f.draft.App.Preset != config.PresetNetworked {
		t.Fatalf("preset label should update, got %q", f.draft.App.Preset)
	}
}

func TestFormToConfigKeepsTypedValues(t *testing.T) {
	f := newForm(config.AppConfig{}, true)
	f.image.SetValue("docker.io/firefox@sha256:abc")
	f.applyPreset(config.PresetNetworked) // must not clobber the typed image
	f.name.SetValue("firefox")

	c := f.toConfig()
	if c.App.Name != "firefox" || c.App.Image != "docker.io/firefox@sha256:abc" {
		t.Fatalf("typed values lost: %+v", c.App)
	}
	if err := config.Validate(c); err != nil {
		t.Fatalf("form config should validate: %v", err)
	}
}

func TestCycle(t *testing.T) {
	opts := []string{config.NetworkNone, config.NetworkPasta, config.NetworkContainer}
	if got := cycle(opts, config.NetworkNone, +1); got != config.NetworkPasta {
		t.Fatalf("cycle +1 = %q, want pasta", got)
	}
	if got := cycle(opts, config.NetworkNone, -1); got != config.NetworkContainer {
		t.Fatalf("cycle -1 should wrap to container, got %q", got)
	}
}

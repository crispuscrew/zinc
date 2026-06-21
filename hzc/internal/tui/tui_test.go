package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/core/adapters/fs"
	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/core/wire"
	"github.com/crispuscrew/hyprzinc/hzc/internal/keys"
)

var errTest = errors.New("bad toml")

func key(spec string) tea.KeyMsg {
	switch spec {
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
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(spec)}
	}
}

func sample(name string) domain.AppConfig {
	cfg, _ := domain.DefaultsFor(domain.PresetStrict)
	cfg.App.Name = name
	cfg.App.Image = "docker.io/library/" + name + "@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return cfg
}

// newLoaded returns a model whose store holds the given apps, already loaded, using
// the default keybind scheme.
func newLoaded(t *testing.T, names ...string) (Model, *fs.Store) {
	t.Helper()
	return loadedWith(t, keys.Active{}, names...)
}

// loadedWith is newLoaded with an explicit keybind scheme (for remap tests). A zero
// keys.Active falls back to the default scheme. The service is wired with the real
// podman adapters; the runtime queries (running set) degrade to empty without podman,
// which is exactly what these UI-logic tests want.
func loadedWith(t *testing.T, active keys.Active, names ...string) (Model, *fs.Store) {
	t.Helper()
	sto := &fs.Store{Root: t.TempDir()}
	for _, name := range names {
		if err := sto.Save(sample(name)); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	svc := wire.Service(sto)
	mdl := New(svc, domain.HostOptions{}, active)
	updated, _ := mdl.Update(loadApps(svc)()) // run the load command synchronously
	return updated.(Model), sto
}

func send(mdl Model, msg tea.Msg) Model {
	updated, _ := mdl.Update(msg)
	return updated.(Model)
}

func TestListNavigation(t *testing.T) {
	mdl, _ := newLoaded(t, "alpha", "beta", "gamma") // List() sorts → alpha,beta,gamma
	if len(mdl.apps) != 3 {
		t.Fatalf("want 3 apps, got %d", len(mdl.apps))
	}
	mdl = send(mdl, key("j"))
	mdl = send(mdl, key("j"))
	if mdl.cursor != 2 {
		t.Fatalf("cursor after 2×down = %d, want 2", mdl.cursor)
	}
	mdl = send(mdl, key("j")) // clamp at bottom
	if mdl.cursor != 2 {
		t.Fatalf("cursor should clamp at 2, got %d", mdl.cursor)
	}
	mdl = send(mdl, key("k"))
	if mdl.cursor != 1 {
		t.Fatalf("cursor after up = %d, want 1", mdl.cursor)
	}
}

func TestListToFormModes(t *testing.T) {
	mdl, _ := newLoaded(t, "alpha")

	create := send(mdl, key("n"))
	if create.mode != modeForm || create.form == nil || !create.form.creating {
		t.Fatalf("n should open a create form")
	}

	edit := send(mdl, key("e"))
	if edit.mode != modeForm || edit.form == nil || edit.form.creating {
		t.Fatalf("e should open an edit form for the selected app")
	}
	if edit.form.draft.App.Name != "alpha" {
		t.Fatalf("edit form should load 'alpha', got %q", edit.form.draft.App.Name)
	}

	back := send(edit, key("esc"))
	if back.mode != modeList || back.form != nil {
		t.Fatalf("esc should return to the list")
	}
}

func TestQuit(t *testing.T) {
	mdl, _ := newLoaded(t)
	quit := send(mdl, key("q"))
	if !quit.quitting {
		t.Fatal("q should set quitting")
	}
}

func TestDeleteFlow(t *testing.T) {
	mdl, sto := newLoaded(t, "alpha", "beta")
	mdl = send(mdl, key("d")) // confirm delete of 'alpha'
	if mdl.mode != modeConfirmDelete || mdl.confirmName != "alpha" {
		t.Fatalf("d should ask to confirm deleting alpha, got mode=%v name=%q", mdl.mode, mdl.confirmName)
	}
	updated, cmd := mdl.Update(key("y"))
	mdl = updated.(Model)
	if mdl.mode != modeList {
		t.Fatalf("after confirm, mode should be list")
	}
	if cmd == nil {
		t.Fatal("confirm should return a remove command")
	}
	cmd() // perform the delete
	if sto.Exists("alpha") {
		t.Fatal("alpha should be deleted from the store")
	}
	if !sto.Exists("beta") {
		t.Fatal("beta should be untouched")
	}
}

func TestSaveInvalidStaysInForm(t *testing.T) {
	sto := &fs.Store{Root: t.TempDir()}
	mdl := New(wire.Service(sto), domain.HostOptions{}, keys.Active{})
	mdl.mode = modeForm
	mdl.form = newForm(domain.AppConfig{}, true)
	mdl.form.name.SetValue("x")
	mdl.form.image.SetValue("alpine:latest") // third-party, not digest-pinned (§5.5)

	mdl = send(mdl, key("ctrl+s"))
	if mdl.mode != modeForm {
		t.Fatal("invalid save should keep the form open")
	}
	if mdl.form == nil || mdl.form.err == nil {
		t.Fatal("invalid save should surface an error in the form")
	}
	if sto.Exists("x") {
		t.Fatal("nothing should be written when validation fails")
	}
}

func TestSaveValid(t *testing.T) {
	sto := &fs.Store{Root: t.TempDir()}
	mdl := New(wire.Service(sto), domain.HostOptions{}, keys.Active{})
	mdl.mode = modeForm
	mdl.form = newForm(domain.AppConfig{}, true)
	mdl.form.name.SetValue("zed")
	mdl.form.image.SetValue("docker.io/zed@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	mdl = send(mdl, key("ctrl+s"))
	if mdl.mode != modeList {
		t.Fatalf("valid save should return to the list, got mode %v", mdl.mode)
	}
	if !sto.Exists("zed") {
		t.Fatal("valid save should persist the app")
	}
}

func TestFormPresetReseed(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true)
	if frm.draft.Network.Mode != domain.NetworkNone || frm.draft.Display.Wayland != domain.WaylandSecurityContext {
		t.Fatalf("create form should start from strict defaults: %+v", frm.draft)
	}
	frm.applyPreset(domain.PresetNetworked)
	if frm.draft.Network.Mode != domain.NetworkPasta || frm.draft.Display.Wayland != domain.WaylandPassthrough {
		t.Fatalf("networked preset should reseed mode/wayland: %+v", frm.draft)
	}
	if frm.draft.App.Preset != domain.PresetNetworked {
		t.Fatalf("preset label should update, got %q", frm.draft.App.Preset)
	}
}

func TestFormToConfigKeepsTypedValues(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true)
	frm.image.SetValue("docker.io/firefox@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	frm.applyPreset(domain.PresetNetworked) // must not clobber the typed image
	frm.name.SetValue("firefox")

	cfg := frm.toConfig()
	if cfg.App.Name != "firefox" || cfg.App.Image != "docker.io/firefox@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("typed values lost: %+v", cfg.App)
	}
	if err := domain.Validate(cfg); err != nil {
		t.Fatalf("form config should validate: %v", err)
	}
}

func fieldIdx(frm *formModel, label string) int {
	for idx, fld := range frm.fields {
		if fld.label == label {
			return idx
		}
	}
	return -1
}

func TestFormClearField(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true)
	frm.focus(fieldIdx(frm, "image"))
	frm.image.SetValue("docker.io/x@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	_, res := frm.update(key("ctrl+d"))
	if res != formStay {
		t.Fatalf("ctrl+d should keep the form, got %v", res)
	}
	if frm.image.Value() != "" {
		t.Fatalf("ctrl+d should clear the focused text field, got %q", frm.image.Value())
	}
}

func TestFormAdvancedTriggersEdit(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true)
	frm.focus(fieldIdx(frm, "advanced"))
	if _, res := frm.update(key("enter")); res != formEdit {
		t.Fatalf("enter on the advanced row should request an editor, got %v", res)
	}
}

func TestFormReloadKeepsCreating(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true) // creating
	edited := sample("edited")
	edited.App.Image = "docker.io/edited@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	edited.Network.Mode = domain.NetworkPasta

	frm.reload(edited)
	if !frm.creating {
		t.Fatal("reload must preserve the creating flag")
	}
	if frm.image.Value() != edited.App.Image {
		t.Fatalf("reload should re-seed the image input, got %q", frm.image.Value())
	}
	if frm.draft.Network.Mode != domain.NetworkPasta {
		t.Fatalf("reload should swap in the edited draft, got mode %q", frm.draft.Network.Mode)
	}
}

func TestEditedMsgReloadsForm(t *testing.T) {
	mdl, _ := newLoaded(t, "alpha")
	mdl = send(mdl, key("e")) // edit form for alpha
	if mdl.mode != modeForm || mdl.form == nil {
		t.Fatal("e should open the edit form")
	}
	cfg := mdl.form.draft
	cfg.App.Image = "docker.io/new@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	updated, _ := mdl.Update(editedMsg{cfg: cfg})
	mdl = updated.(Model)
	if mdl.form == nil || mdl.form.image.Value() != cfg.App.Image {
		t.Fatalf("editedMsg should reload the form with the edited image")
	}
}

func TestEditedMsgErrorStaysInForm(t *testing.T) {
	mdl, _ := newLoaded(t, "alpha")
	mdl = send(mdl, key("e"))
	updated, _ := mdl.Update(editedMsg{err: errTest})
	mdl = updated.(Model)
	if mdl.mode != modeForm || mdl.form == nil || mdl.form.err == nil {
		t.Fatal("a bad edit should keep the form open and surface the error")
	}
}

func TestFormTerminalToggle(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true)
	frm.focus(fieldIdx(frm, "terminal"))
	if frm.draft.App.Terminal {
		t.Fatal("terminal should default off")
	}
	frm.update(key("enter"))
	if !frm.draft.App.Terminal {
		t.Fatal("enter on the terminal row should toggle app.terminal on")
	}
}

func TestFormResolveKey(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true)
	frm.focus(fieldIdx(frm, "image"))
	if _, res := frm.update(tea.KeyMsg{Type: tea.KeyCtrlR}); res != formResolve {
		t.Fatalf("ctrl+r on the image field should request resolve, got %v", res)
	}
	frm.focus(fieldIdx(frm, "description"))
	if _, res := frm.update(tea.KeyMsg{Type: tea.KeyCtrlR}); res != formStay {
		t.Fatalf("ctrl+r off the image field should be a no-op, got %v", res)
	}
}

func TestResolvedMsgUpdatesImage(t *testing.T) {
	mdl, _ := newLoaded(t, "alpha")
	mdl = send(mdl, key("e"))
	updated, _ := mdl.Update(resolvedMsg{ref: "docker.io/library/alpha@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"})
	mdl = updated.(Model)
	if mdl.form == nil || mdl.form.image.Value() != "docker.io/library/alpha@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd" {
		t.Fatalf("resolvedMsg should set the image field, got %q", mdl.form.image.Value())
	}
}

func TestFormHidesNetworkTarget(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true) // strict default → network mode none
	if fieldIdx(frm, "network.target") != -1 {
		t.Fatal("network.target must be hidden when mode != container")
	}
	frm.focus(fieldIdx(frm, "network.mode"))
	frm.update(key("right")) // none → pasta
	frm.update(key("right")) // pasta → container
	if frm.draft.Network.Mode != domain.NetworkContainer {
		t.Fatalf("expected container mode after two cycles, got %q", frm.draft.Network.Mode)
	}
	if fieldIdx(frm, "network.target") == -1 {
		t.Fatal("network.target must appear when mode == container")
	}
}

func TestFormClearsTargetWhenNotContainer(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true)
	frm.target.SetValue("vpn-container") // stale value from a prior container selection
	frm.draft.Network.Mode = domain.NetworkNone
	if got := frm.toConfig().Network.Target; got != "" {
		t.Fatalf("target must be cleared when mode != container, got %q", got)
	}
}

// TestSchemeDrivesListKeys proves list keys come from the active scheme, not
// hardcoded literals: "g" refreshes under default but is unbound under vim.
func TestSchemeDrivesListKeys(t *testing.T) {
	def, _ := newLoaded(t, "alpha", "beta")
	if _, cmd := def.Update(key("g")); cmd == nil {
		t.Fatal("default scheme: 'g' should trigger refresh")
	}
	vim, _ := loadedWith(t, keys.Active{Name: "vim", Scheme: keys.Vim}, "alpha", "beta")
	if _, cmd := vim.Update(key("g")); cmd != nil {
		t.Fatal("vim scheme: 'g' should be unbound in the list")
	}
	vim = send(vim, key("j")) // vim still navigates with j
	if vim.cursor != 1 {
		t.Fatalf("vim scheme: 'j' should move down, cursor=%d", vim.cursor)
	}
}

// TestSchemeRemapsFormNav proves the form resolves keys through the scheme too: vim
// binds ctrl+n to next-field; default does not.
func TestSchemeRemapsFormNav(t *testing.T) {
	vimForm := newForm(domain.AppConfig{}, true)
	vimForm.scheme = keys.Vim
	startIdx := vimForm.idx
	vimForm.update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if vimForm.idx == startIdx {
		t.Fatal("vim: ctrl+n should advance to the next field")
	}
	defForm := newForm(domain.AppConfig{}, true) // nil scheme → default bindings
	defStart := defForm.idx
	defForm.update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if defForm.idx != defStart {
		t.Fatal("default: ctrl+n should be unbound in the form")
	}
}

func TestKeysPickerOpens(t *testing.T) {
	mdl, _ := newLoaded(t, "alpha")
	updated, cmd := mdl.Update(key("?")) // default binds "?" to the scheme picker
	mdl = updated.(Model)
	if mdl.mode != modeKeys {
		t.Fatalf("'?' should open the scheme picker, mode=%v", mdl.mode)
	}
	if cmd == nil {
		t.Fatal("opening the picker should request the scheme list")
	}
	mdl = send(mdl, schemesMsg{names: []string{"default", "vim"}})
	if len(mdl.keysList) != 2 {
		t.Fatalf("picker should hold 2 schemes, got %d", len(mdl.keysList))
	}
	mdl = send(mdl, key("j"))
	if mdl.keysCursor != 1 {
		t.Fatalf("picker nav should move the cursor, got %d", mdl.keysCursor)
	}
	mdl = send(mdl, key("esc"))
	if mdl.mode != modeList {
		t.Fatal("esc should leave the picker")
	}
}

// The Shell action opens another terminal only for a multiterminal app; for any
// other app it is a no-op with an explanatory status.
func TestShellActionMultiterminalOnly(t *testing.T) {
	sto := &fs.Store{Root: t.TempDir()}
	multi := sample("alpha") // sorts first → selected at start
	multi.App.Terminal = true
	multi.App.Multiterminal = true
	multi.App.Command = []string{"htop"}
	for _, cfg := range []domain.AppConfig{multi, sample("beta")} {
		if err := sto.Save(cfg); err != nil {
			t.Fatalf("seed %s: %v", cfg.App.Name, err)
		}
	}
	svc := wire.Service(sto)
	mdl := New(svc, domain.HostOptions{}, keys.Active{})
	updated, _ := mdl.Update(loadApps(svc)())
	mdl = updated.(Model)

	if _, cmd := mdl.Update(key("S")); cmd == nil {
		t.Fatal("shell on a multiterminal app should return a command")
	}
	mdl = send(mdl, key("j"))
	updated, cmd := mdl.Update(key("S"))
	if cmd != nil {
		t.Fatal("shell on a non-multiterminal app should be a no-op")
	}
	if got := updated.(Model).status; !strings.Contains(got, "multiterminal") {
		t.Fatalf("expected a multiterminal hint in the status, got %q", got)
	}
}

// The Build action needs an install line: it returns a build command for an app with
// app.install, and is a no-op (with a status hint) for one without.
func TestBuildActionRequiresInstall(t *testing.T) {
	sto := &fs.Store{Root: t.TempDir()}
	withInstall := sample("alpha") // sorts first → selected at start
	withInstall.App.Install = "apk add --no-cache sl"
	for _, cfg := range []domain.AppConfig{withInstall, sample("beta")} {
		if err := sto.Save(cfg); err != nil {
			t.Fatalf("seed %s: %v", cfg.App.Name, err)
		}
	}
	svc := wire.Service(sto)
	mdl := New(svc, domain.HostOptions{}, keys.Active{})
	updated, _ := mdl.Update(loadApps(svc)())
	mdl = updated.(Model)

	if _, cmd := mdl.Update(key("b")); cmd == nil {
		t.Fatal("build on an app with an install line should return a command")
	}
	mdl = send(mdl, key("j")) // move to beta (no install)
	updated, cmd := mdl.Update(key("b"))
	if cmd != nil {
		t.Fatal("build on an app without an install line should be a no-op")
	}
	if got := updated.(Model).status; !strings.Contains(got, "nothing to build") {
		t.Fatalf("expected a 'nothing to build' status, got %q", got)
	}
}

// The form's entrypoint + install fields fold into App.Command / App.Install: a
// space-separated command splits to argv, and the install line passes through.
func TestFormCommandAndInstallRoundtrip(t *testing.T) {
	frm := newForm(domain.AppConfig{}, true)
	frm.name.SetValue("hollywood")
	frm.image.SetValue("docker.io/library/debian@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	frm.command.SetValue("  hollywood --speed 2 ")
	frm.install.SetValue("  apt-get update && apt-get install -y hollywood ")

	cfg := frm.toConfig()
	if got := cfg.App.Command; len(got) != 3 || got[0] != "hollywood" || got[2] != "2" {
		t.Fatalf("command should split on whitespace, got %v", got)
	}
	if cfg.App.Install != "apt-get update && apt-get install -y hollywood" {
		t.Fatalf("install line should be trimmed and kept, got %q", cfg.App.Install)
	}
	if err := domain.Validate(cfg); err != nil {
		t.Fatalf("form config should validate: %v", err)
	}
}

// Editing an app without touching the command field must not flatten a complex argv
// (quoted, multi-word) that was authored via the advanced TOML editor.
func TestFormCommandPreservesComplexArgv(t *testing.T) {
	cfg := sample("svc")
	cfg.App.Command = []string{"sh", "-c", "echo hi"} // 3 elements; field shows "sh -c echo hi"
	frm := newForm(cfg, false)
	if got := frm.toConfig().App.Command; len(got) != 3 {
		t.Fatalf("untouched command must be preserved verbatim, got %v", got)
	}
}

func TestCycle(t *testing.T) {
	opts := []string{domain.NetworkNone, domain.NetworkPasta, domain.NetworkContainer}
	if got := cycle(opts, domain.NetworkNone, +1); got != domain.NetworkPasta {
		t.Fatalf("cycle +1 = %q, want pasta", got)
	}
	if got := cycle(opts, domain.NetworkNone, -1); got != domain.NetworkContainer {
		t.Fatalf("cycle -1 should wrap to container, got %q", got)
	}
}

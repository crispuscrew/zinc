package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/creator/internal/backend"
	"github.com/crispuscrew/zinc/container/creator/internal/keys"
	"github.com/crispuscrew/zinc/container/creator/internal/store"
)

var errTest = errors.New("bad yaml")

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

// img is a valid digest-pinned reference for a name (section 5.5).
func img(name string) string {
	return "docker.io/library/" + name + "@sha256:" + strings.Repeat("a", 64)
}

func sample(name string) schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     name,
		ImageMeta:     schema.ImageMeta{Image: img(name)},
	}
}

// newLoaded returns a model whose store holds the given apps, already loaded, using the
// default keybind scheme.
func newLoaded(t *testing.T, names ...string) (Model, *store.Store) {
	t.Helper()
	return loadedWith(t, keys.Active{}, names...)
}

// loadedWith is newLoaded with an explicit keybind scheme (for remap tests). A zero
// keys.Active falls back to the default scheme. The backend delegates runtime queries
// (the running set) to zcr, which is absent under test; that degrades to "nothing
// running", which is exactly what these UI-logic tests want.
func loadedWith(t *testing.T, active keys.Active, names ...string) (Model, *store.Store) {
	t.Helper()
	sto := &store.Store{Root: t.TempDir()}
	for _, name := range names {
		if err := sto.Save(sample(name)); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	svc := backend.New(sto)
	mdl := New(svc, active)
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
	if edit.form.draft.AppNameID != "alpha" {
		t.Fatalf("edit form should load 'alpha', got %q", edit.form.draft.AppNameID)
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

func TestRenameFlow(t *testing.T) {
	mdl, sto := newLoaded(t, "alpha", "beta") // cursor starts on alpha
	mdl = send(mdl, key("R"))                 // open the rename prompt for alpha
	if mdl.mode != modeRename || mdl.renameFrom != "alpha" {
		t.Fatalf("R should open the rename prompt for alpha, got mode=%v from=%q", mdl.mode, mdl.renameFrom)
	}
	mdl.rename.SetValue("delta")
	updated, cmd := mdl.Update(key("enter"))
	mdl = updated.(Model)
	if mdl.mode != modeList {
		t.Fatal("after confirm, mode should return to the list")
	}
	if cmd == nil {
		t.Fatal("confirming a rename should return a command")
	}
	cmd() // perform the rename
	if sto.Exists("alpha") {
		t.Fatal("the old name should be gone after rename")
	}
	if !sto.Exists("delta") {
		t.Fatal("the new name should exist after rename")
	}
	if !sto.Exists("beta") {
		t.Fatal("an unrelated app must be untouched")
	}
}

// Pressing enter without changing the name is a harmless no-op, not a destructive
// delete-and-recreate.
func TestRenameUnchangedIsNoOp(t *testing.T) {
	mdl, sto := newLoaded(t, "alpha")
	mdl = send(mdl, key("R"))
	updated, cmd := mdl.Update(key("enter")) // value still "alpha"
	mdl = updated.(Model)
	if mdl.mode != modeList || cmd != nil {
		t.Fatal("an unchanged rename should just return to the list with no command")
	}
	if !sto.Exists("alpha") {
		t.Fatal("alpha must still exist")
	}
}

func TestSaveInvalidStaysInForm(t *testing.T) {
	sto := &store.Store{Root: t.TempDir()}
	mdl := New(backend.New(sto), keys.Active{})
	mdl.mode = modeForm
	mdl.form = newForm(schema.AppConfig{}, true)
	mdl.form.name.SetValue("x")
	mdl.form.image.SetValue("alpine:latest") // third-party, not digest-pinned (section 5.5)

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
	sto := &store.Store{Root: t.TempDir()}
	mdl := New(backend.New(sto), keys.Active{})
	mdl.mode = modeForm
	mdl.form = newForm(schema.AppConfig{}, true)
	mdl.form.name.SetValue("zed")
	mdl.form.image.SetValue(img("zed"))

	mdl = send(mdl, key("ctrl+s"))
	if mdl.mode != modeList {
		t.Fatalf("valid save should return to the list, got mode %v", mdl.mode)
	}
	if !sto.Exists("zed") {
		t.Fatal("valid save should persist the app")
	}
}

func TestFormToConfigKeepsTypedValues(t *testing.T) {
	frm := newForm(schema.AppConfig{}, true)
	frm.image.SetValue(img("firefox"))
	frm.name.SetValue("firefox")

	cfg := frm.toConfig()
	if cfg.AppNameID != "firefox" || cfg.ImageMeta.Image != img("firefox") {
		t.Fatalf("typed values lost: name=%q image=%q", cfg.AppNameID, cfg.ImageMeta.Image)
	}
	if cfg.SchemaVersion != schema.SchemaVersion || cfg.Type != schema.ZincContainer {
		t.Fatalf("form config should carry the schema version + type: %+v", cfg)
	}
	if err := validate.Validate(cfg); err != nil {
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
	frm := newForm(schema.AppConfig{}, true)
	frm.focus(fieldIdx(frm, "image"))
	frm.image.SetValue(img("x"))

	_, res := frm.update(key("ctrl+d"))
	if res != formStay {
		t.Fatalf("ctrl+d should keep the form, got %v", res)
	}
	if frm.image.Value() != "" {
		t.Fatalf("ctrl+d should clear the focused text field, got %q", frm.image.Value())
	}
}

// The entrypoint field folds to StartConditions.Entrypoint (a trimmed command line) and
// the multi-line install folds to ImageMeta.Install (one entry per non-blank line).
func TestFormEntrypointAndInstall(t *testing.T) {
	frm := newForm(schema.AppConfig{}, true)
	frm.name.SetValue("hollywood")
	frm.image.SetValue(img("debian"))
	frm.entrypoint.SetValue("  hollywood --speed 2 ")
	frm.install.SetValue("  apt-get update \n apt-get install -y hollywood \n\n")

	cfg := frm.toConfig()
	if cfg.StartConditions.Entrypoint != "hollywood --speed 2" {
		t.Fatalf("entrypoint should be trimmed and kept as a line, got %q", cfg.StartConditions.Entrypoint)
	}
	if got := cfg.ImageMeta.Install; len(got) != 2 || got[0] != "apt-get update" || got[1] != "apt-get install -y hollywood" {
		t.Fatalf("install should split to non-blank lines, got %v", got)
	}
	if err := validate.Validate(cfg); err != nil {
		t.Fatalf("form config should validate: %v", err)
	}
}

func TestFormAdvancedTriggersEdit(t *testing.T) {
	frm := newForm(schema.AppConfig{}, true)
	frm.focus(fieldIdx(frm, "advanced"))
	if _, res := frm.update(key("enter")); res != formEdit {
		t.Fatalf("enter on the advanced row should request an editor, got %v", res)
	}
}

func TestFormReloadKeepsCreating(t *testing.T) {
	frm := newForm(schema.AppConfig{}, true) // creating
	edited := sample("edited")
	edited.ImageMeta.Image = "docker.io/edited@sha256:" + strings.Repeat("d", 64)
	edited.HostTheme = true

	frm.reload(edited)
	if !frm.creating {
		t.Fatal("reload must preserve the creating flag")
	}
	if frm.image.Value() != edited.ImageMeta.Image {
		t.Fatalf("reload should re-seed the image input, got %q", frm.image.Value())
	}
	if !frm.draft.HostTheme {
		t.Fatal("reload should swap in the edited draft (HostTheme)")
	}
}

func TestEditedMsgReloadsForm(t *testing.T) {
	mdl, _ := newLoaded(t, "alpha")
	mdl = send(mdl, key("e")) // edit form for alpha
	if mdl.mode != modeForm || mdl.form == nil {
		t.Fatal("e should open the edit form")
	}
	cfg := mdl.form.draft
	cfg.ImageMeta.Image = img("new")

	updated, _ := mdl.Update(editedMsg{cfg: cfg})
	mdl = updated.(Model)
	if mdl.form == nil || mdl.form.image.Value() != cfg.ImageMeta.Image {
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
	frm := newForm(schema.AppConfig{}, true)
	frm.focus(fieldIdx(frm, "terminal"))
	if frm.draft.StartConditions.Terminal {
		t.Fatal("terminal should default off")
	}
	frm.update(key("enter"))
	if !frm.draft.StartConditions.Terminal {
		t.Fatal("enter on the terminal row should toggle StartConditions.Terminal on")
	}
}

func TestFormResolveKey(t *testing.T) {
	frm := newForm(schema.AppConfig{}, true)
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
	pinned := "docker.io/library/alpha@sha256:" + strings.Repeat("d", 64)
	updated, _ := mdl.Update(resolvedMsg{ref: pinned})
	mdl = updated.(Model)
	if mdl.form == nil || mdl.form.image.Value() != pinned {
		t.Fatalf("resolvedMsg should set the image field, got %q", mdl.form.image.Value())
	}
}

// TestSchemeDrivesListKeys proves list keys come from the active scheme, not hardcoded
// literals: "g" refreshes under default but is unbound under vim.
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
	vimForm := newForm(schema.AppConfig{}, true)
	vimForm.scheme = keys.Vim
	startIdx := vimForm.idx
	vimForm.update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if vimForm.idx == startIdx {
		t.Fatal("vim: ctrl+n should advance to the next field")
	}
	defForm := newForm(schema.AppConfig{}, true) // nil scheme → default bindings
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

// The Shell action opens another terminal only for a multiterminal app; for any other
// app it is a no-op with an explanatory status.
func TestShellActionMultiterminalOnly(t *testing.T) {
	sto := &store.Store{Root: t.TempDir()}
	multi := sample("alpha") // sorts first → selected at start
	multi.StartConditions.Terminal = true
	multi.StartConditions.Multiterminal = true
	multi.StartConditions.Entrypoint = "htop"
	for _, cfg := range []schema.AppConfig{multi, sample("beta")} {
		if err := sto.Save(cfg); err != nil {
			t.Fatalf("seed %s: %v", cfg.AppNameID, err)
		}
	}
	svc := backend.New(sto)
	mdl := New(svc, keys.Active{})
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

// The Build action needs install lines: it returns a build command for an app with
// ImageMeta.Install, and is a no-op (with a status hint) for one without.
func TestBuildActionRequiresInstall(t *testing.T) {
	sto := &store.Store{Root: t.TempDir()}
	withInstall := sample("alpha") // sorts first → selected at start
	withInstall.ImageMeta.Install = []string{"apk add --no-cache sl"}
	for _, cfg := range []schema.AppConfig{withInstall, sample("beta")} {
		if err := sto.Save(cfg); err != nil {
			t.Fatalf("seed %s: %v", cfg.AppNameID, err)
		}
	}
	svc := backend.New(sto)
	mdl := New(svc, keys.Active{})
	updated, _ := mdl.Update(loadApps(svc)())
	mdl = updated.(Model)

	if _, cmd := mdl.Update(key("b")); cmd == nil {
		t.Fatal("build on an app with install lines should return a command")
	}
	mdl = send(mdl, key("j")) // move to beta (no install)
	updated, cmd := mdl.Update(key("b"))
	if cmd != nil {
		t.Fatal("build on an app without install lines should be a no-op")
	}
	if got := updated.(Model).status; !strings.Contains(got, "nothing to build") {
		t.Fatalf("expected a 'nothing to build' status, got %q", got)
	}
}

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/creator/internal/keys"
)

// formResult is what a form key-press tells the parent to do.
type formResult int

const (
	formStay formResult = iota
	formSave
	formCancel
	formEdit    // open the draft in $EDITOR (the "advanced" action)
	formResolve // resolve the image field's tag to a pinned @sha256 digest
)

type fieldKind int

const (
	kindText      fieldKind = iota // free text via a textinput
	kindMultiline                  // multi-line free text via a textarea (ImageMeta.Install)
	kindBool                       // toggle
	kindInfo                       // read-only (shown, not navigable)
	kindAction                     // navigable; enter triggers an action (e.g. edit YAML)
)

// formField is one editable (or informational) row. Closures read/write the form's
// draft config so the rendered value is always the actual value.
type formField struct {
	label string
	kind  fieldKind
	input *textinput.Model
	area  *textarea.Model
	get   func() string
	set   func(string)
	bget  func() bool
	bset  func(bool)
	info  func() string
}

// formModel is the create/edit form. draft holds the bool values directly; the text
// fields live in their own inputs and are folded into draft on save. The v2 schema's
// list-valued fields (ImageMeta.Install is line-oriented here; Capabilities,
// NetworkMeta.NetworkLists, Volumes, Configs, Keys) are edited via the advanced $EDITOR
// action, summarized on the "advanced" row.
type formModel struct {
	creating bool
	draft    schema.AppConfig
	scheme   keys.Scheme // nil → default bindings (keys.Scheme handles the fallback)

	name       textinput.Model
	image      textinput.Model
	entrypoint textinput.Model // StartConditions.Entrypoint (space-separated command line)
	install    textarea.Model  // ImageMeta.Install, one shell line per row → derived image
	desc       textinput.Model
	icon       textinput.Model

	fields []formField
	idx    int
	err    error
}

func newForm(base schema.AppConfig, creating bool) *formModel {
	frm := &formModel{creating: creating, draft: base}
	if frm.draft.SchemaVersion == 0 {
		frm.draft.SchemaVersion = schema.SchemaVersion
	}
	if frm.draft.Type == "" {
		frm.draft.Type = schema.ZincContainer
	}

	frm.name = newInput(frm.draft.AppNameID, "firefox")
	frm.image = newInput(frm.draft.ImageMeta.Image, "docker.io/…@sha256:… (trusted images may use a tag)")
	frm.entrypoint = newInput(frm.draft.StartConditions.Entrypoint, "entrypoint, e.g. firefox (blank = image default)")
	frm.install = newArea(strings.Join(frm.draft.ImageMeta.Install, "\n"), "build setup, one shell line per row, e.g. apt-get install -y firefox (blank = none)")
	frm.desc = newInput(frm.draft.Description, "")
	frm.icon = newInput(frm.draft.Icon, "freedesktop name (e.g. firefox) or /path/to/icon.png")

	frm.buildFields()
	frm.idx = -1
	frm.focusNext() // land on the first editable field
	return frm
}

func newInput(value, placeholder string) textinput.Model {
	inp := textinput.New()
	inp.Prompt = ""
	inp.Placeholder = placeholder
	inp.CharLimit = 256
	inp.SetValue(value)
	return inp
}

// newArea builds the multi-line install editor. Each non-empty line becomes one entry
// of ImageMeta.Install (the build's RUN steps), so the user can lay a multi-step setup
// out across lines.
func newArea(value, placeholder string) textarea.Model {
	area := textarea.New()
	area.Prompt = ""
	area.Placeholder = placeholder
	area.ShowLineNumbers = false
	area.CharLimit = 1024
	area.SetWidth(64)
	area.SetHeight(3)
	area.SetValue(value)
	area.Blur()
	return area
}

func (frm *formModel) buildFields() {
	boolean := func(label string, get func() bool, set func(bool)) formField {
		return formField{label: label, kind: kindBool, bget: get, bset: set}
	}

	var fields []formField
	if frm.creating {
		fields = append(fields, formField{label: "name", kind: kindText, input: &frm.name})
	} else {
		fields = append(fields, formField{label: "name", kind: kindInfo,
			info: func() string { return frm.draft.AppNameID + "   (rename with R in the list)" }})
	}
	fields = append(fields,
		formField{label: "image", kind: kindText, input: &frm.image},
		// Quick-setup fields, grouped with the image they derive from (§9.1): the
		// entrypoint to run, and build-time install lines that produce a derived image
		// (FROM image + RUN install).
		formField{label: "entrypoint", kind: kindText, input: &frm.entrypoint},
		formField{label: "install", kind: kindMultiline, area: &frm.install},
		formField{label: "description", kind: kindText, input: &frm.desc},
		formField{label: "icon", kind: kindText, input: &frm.icon},
		boolean("terminal",
			func() bool { return frm.draft.StartConditions.Terminal },
			func(val bool) { frm.draft.StartConditions.Terminal = val }),
		boolean("multiterminal",
			func() bool { return frm.draft.StartConditions.Multiterminal },
			func(val bool) { frm.draft.StartConditions.Multiterminal = val }),
		boolean("autorestart",
			func() bool { return frm.draft.StartConditions.Autorestart },
			func(val bool) { frm.draft.StartConditions.Autorestart = val }),
		boolean("keep_alive",
			func() bool { return frm.draft.StopConditions.KeepAlive },
			func(val bool) { frm.draft.StopConditions.KeepAlive = val }),
		boolean("background",
			func() bool { return frm.draft.StopConditions.Background },
			func(val bool) { frm.draft.StopConditions.Background = val }),
		boolean("display.disable_gpu",
			func() bool { return frm.draft.DisplayMeta.DisableGpuAccess },
			func(val bool) { frm.draft.DisplayMeta.DisableGpuAccess = val }),
		boolean("display.disable_security_context",
			func() bool { return frm.draft.DisplayMeta.DisableSecurityContext },
			func(val bool) { frm.draft.DisplayMeta.DisableSecurityContext = val }),
		boolean("audio.pipewire",
			func() bool { return frm.draft.AudioMeta.Pipewire },
			func(val bool) { frm.draft.AudioMeta.Pipewire = val }),
		boolean("audio.legacy_alsa",
			func() bool { return frm.draft.AudioMeta.LegacyALSA },
			func(val bool) { frm.draft.AudioMeta.LegacyALSA = val }),
		boolean("host_theme",
			func() bool { return frm.draft.HostTheme },
			func(val bool) { frm.draft.HostTheme = val }),
		formField{label: "advanced", kind: kindAction, info: frm.advancedSummary},
	)
	frm.fields = fields
}

// reload swaps in a config edited out-of-band (the $EDITOR round-trip) and re-seeds the
// text inputs from it, keeping the same form (and its creating flag) so a half-finished
// new app isn't reset.
func (frm *formModel) reload(cfg schema.AppConfig) {
	frm.draft = cfg
	if frm.draft.SchemaVersion == 0 {
		frm.draft.SchemaVersion = schema.SchemaVersion
	}
	if frm.draft.Type == "" {
		frm.draft.Type = schema.ZincContainer
	}
	frm.name.SetValue(cfg.AppNameID)
	frm.image.SetValue(cfg.ImageMeta.Image)
	frm.entrypoint.SetValue(cfg.StartConditions.Entrypoint)
	frm.install.SetValue(strings.Join(cfg.ImageMeta.Install, "\n"))
	frm.desc.SetValue(cfg.Description)
	frm.icon.SetValue(cfg.Icon)
	frm.buildFields()
	frm.err = nil
	if frm.idx >= len(frm.fields) {
		frm.idx = len(frm.fields) - 1
	}
	frm.focus(frm.idx)
}

// advancedSummary counts the list-valued fields the form doesn't edit inline, pointing
// at the $EDITOR escape hatch for them.
func (frm *formModel) advancedSummary() string {
	return fmt.Sprintf("install=%d  caps=%d  networks=%d  volumes=%d configs=%d  keys=%d  depends_on=%d   (enter: edit YAML in $EDITOR)",
		len(frm.draft.ImageMeta.Install), len(frm.draft.Capabilities), len(frm.draft.NetworkMeta.NetworkLists),
		len(frm.draft.Volumes), len(frm.draft.Configs), len(frm.draft.Keys), len(frm.draft.StartConditions.DependsOn))
}

func (frm *formModel) update(msg tea.Msg) (tea.Cmd, formResult) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, formStay
	}
	keyStr := keyMsg.String()
	scheme := frm.scheme

	// In the multi-line install field the arrow keys move the cursor within the
	// textarea (intrinsic, like the logs viewport's scrolling); tab / shift+tab still
	// move between fields. Without this, up/down would leave the field — they are the
	// NextField/PrevField bindings.
	if frm.fields[frm.idx].kind == kindMultiline && (keyStr == "up" || keyStr == "down") {
		var cmd tea.Cmd
		frm.install, cmd = frm.install.Update(msg)
		return cmd, formStay
	}

	// Field-kind-independent commands first. These are control keys (esc, ctrl+*, tab),
	// so they never collide with typing into a focused text field.
	switch {
	case scheme.Is(keys.CtxForm, keys.Cancel, keyStr):
		return nil, formCancel
	case scheme.Is(keys.CtxForm, keys.Save, keyStr):
		return nil, formSave
	case scheme.Is(keys.CtxForm, keys.NextField, keyStr):
		frm.focusNext()
		return nil, formStay
	case scheme.Is(keys.CtxForm, keys.PrevField, keyStr):
		frm.focusPrev()
		return nil, formStay
	case scheme.Is(keys.CtxForm, keys.ClearField, keyStr):
		switch cur := frm.fields[frm.idx]; {
		case cur.kind == kindText && cur.input != nil:
			cur.input.SetValue("")
		case cur.kind == kindMultiline && cur.area != nil:
			cur.area.SetValue("")
		}
		return nil, formStay
	case scheme.Is(keys.CtxForm, keys.ResolveImage, keyStr):
		if cur := frm.fields[frm.idx]; cur.kind == kindText && cur.label == "image" {
			return nil, formResolve // pin the typed tag to its @sha256 digest
		}
		return nil, formStay
	}

	// Everything else depends on the focused field's kind. The scheme decides which keys
	// fire each gesture; the dispatch by kind stays here.
	fld := frm.fields[frm.idx]
	switch fld.kind {
	case kindText:
		var cmd tea.Cmd
		*fld.input, cmd = fld.input.Update(msg)
		return cmd, formStay
	case kindMultiline:
		// Enter inserts a newline for a multi-step install script; the lines become the
		// ImageMeta.Install entries on save.
		var cmd tea.Cmd
		*fld.area, cmd = fld.area.Update(msg)
		return cmd, formStay
	case kindAction:
		if scheme.Is(keys.CtxForm, keys.Activate, keyStr) {
			return nil, formEdit
		}
	case kindBool:
		if scheme.Is(keys.CtxForm, keys.Toggle, keyStr) {
			fld.bset(!fld.bget())
		}
	}
	return nil, formStay
}

func (frm *formModel) focus(idx int) {
	frm.idx = idx
	for _, input := range []*textinput.Model{&frm.name, &frm.image, &frm.entrypoint, &frm.desc, &frm.icon} {
		input.Blur()
	}
	frm.install.Blur()
	switch fld := frm.fields[idx]; {
	case fld.kind == kindText && fld.input != nil:
		fld.input.Focus()
	case fld.kind == kindMultiline:
		frm.install.Focus()
	}
}

func (frm *formModel) focusNext() {
	for step := 0; step < len(frm.fields); step++ {
		idx := (frm.idx + 1 + step) % len(frm.fields)
		if frm.fields[idx].kind != kindInfo {
			frm.focus(idx)
			return
		}
	}
}

func (frm *formModel) focusPrev() {
	for step := 0; step < len(frm.fields); step++ {
		idx := ((frm.idx-1-step)%len(frm.fields) + len(frm.fields)) % len(frm.fields)
		if frm.fields[idx].kind != kindInfo {
			frm.focus(idx)
			return
		}
	}
}

// toConfig folds the text inputs into the draft and returns the config to save.
func (frm *formModel) toConfig() schema.AppConfig {
	cfg := frm.draft
	if frm.creating {
		cfg.AppNameID = strings.TrimSpace(frm.name.Value())
	}
	cfg.ImageMeta.Image = strings.TrimSpace(frm.image.Value())
	cfg.StartConditions.Entrypoint = strings.TrimSpace(frm.entrypoint.Value())
	cfg.ImageMeta.Install = splitLines(frm.install.Value())
	cfg.Description = frm.desc.Value()
	cfg.Icon = strings.TrimSpace(frm.icon.Value())
	cfg.SchemaVersion = schema.SchemaVersion
	if cfg.Type == "" {
		cfg.Type = schema.ZincContainer
	}
	return cfg
}

// splitLines turns the install textarea's text into one entry per non-blank line,
// returning nil (not an empty slice) when blank so a cleared install marshals away.
func splitLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

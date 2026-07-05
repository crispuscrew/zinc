package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/hzc/internal/keys"
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
	kindMultiline                  // multi-line free text via a textarea (app.install)
	kindEnum                       // cycle through a fixed option set
	kindBool                       // toggle
	kindInfo                       // read-only (shown, not navigable)
	kindAction                     // navigable; enter triggers an action (e.g. edit TOML)
)

// formField is one editable (or informational) row. Closures read/write the form's
// draft config so the rendered value is always the actual value (§4).
type formField struct {
	label   string
	kind    fieldKind
	input   *textinput.Model
	area    *textarea.Model
	options []string
	get     func() string
	set     func(string)
	bget    func() bool
	bset    func(bool)
	info    func() string
}

// formModel is the create/edit form. draft holds the enum/bool values directly; the
// text fields live in their own inputs and are folded into draft on save.
type formModel struct {
	creating bool
	draft    domain.AppConfig
	scheme   keys.Scheme // nil → default bindings (keys.Scheme handles the fallback)

	name    textinput.Model
	image   textinput.Model
	command textinput.Model // entrypoint argv, space-separated (folded into App.Command)
	install textarea.Model  // build-time setup, possibly multi-line (App.Install) → derived image
	desc    textinput.Model
	icon    textinput.Model
	target  textinput.Model

	fields []formField
	idx    int
	err    error
}

func newForm(base domain.AppConfig, creating bool) *formModel {
	if creating {
		base, _ = domain.DefaultsFor(domain.PresetStrict)
	}
	frm := &formModel{creating: creating, draft: base}
	if frm.draft.SchemaVersion == 0 {
		frm.draft.SchemaVersion = domain.SchemaVersion
	}
	if frm.draft.App.Preset == "" {
		frm.draft.App.Preset = domain.PresetStrict // label only; never enforced (§4)
	}

	frm.name = newInput(frm.draft.App.Name, "firefox")
	frm.image = newInput(frm.draft.App.Image, "docker.io/…@sha256:… (trusted-* may use a tag)")
	frm.command = newInput(strings.Join(frm.draft.App.Command, " "), "entrypoint, e.g. hollywood (blank = image default)")
	frm.install = newArea(frm.draft.App.Install, "build setup, e.g. apt-get install -y hollywood (blank = none)")
	frm.desc = newInput(frm.draft.App.Description, "")
	frm.icon = newInput(frm.draft.App.Icon, "freedesktop name (e.g. firefox) or /path/to/icon.png")
	frm.target = newInput(frm.draft.Network.Target, "vpn-container")

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

// newArea builds the multi-line install editor. Install lines are joined into one
// RUN at build time (domain.installScript), so the user can lay a multi-step setup
// out across lines and still get a single derived layer.
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
	enum := func(label string, opts []string, get func() string, set func(string)) formField {
		return formField{label: label, kind: kindEnum, options: opts, get: get, set: set}
	}
	boolean := func(label string, get func() bool, set func(bool)) formField {
		return formField{label: label, kind: kindBool, bget: get, bset: set}
	}

	var fields []formField
	if frm.creating {
		fields = append(fields, formField{label: "name", kind: kindText, input: &frm.name})
	} else {
		fields = append(fields, formField{label: "name", kind: kindInfo,
			info: func() string { return frm.draft.App.Name + "   (rename with R in the list)" }})
	}
	fields = append(fields,
		formField{label: "image", kind: kindText, input: &frm.image},
		// Quick-setup fields, grouped with the image they derive from (§9.1): the
		// entrypoint to run, and a build-time install line that produces a derived
		// image (FROM image + RUN install). The hint reads the current image live.
		formField{label: "command", kind: kindText, input: &frm.command},
		formField{label: "install", kind: kindMultiline, area: &frm.install},
		formField{label: "", kind: kindInfo, info: func() string {
			// "b rebuilds" is a LIST action — in a text field b just types a "b" — so the
			// form hint only describes what install does, plus the apply gesture (enter)
			// while the field is still empty.
			note := "builds a derived image"
			if strings.TrimSpace(frm.install.Value()) == "" {
				note = "enter applies · " + note
			}
			return "↳ " + domain.InstallHint(strings.TrimSpace(frm.image.Value())) + "  (" + note + ")"
		}},
		enum("preset", []string{domain.PresetStrict, domain.PresetStandard, domain.PresetNetworked},
			func() string { return frm.draft.App.Preset },
			func(val string) { frm.applyPreset(val) }),
		enum("display.wayland", []string{domain.WaylandSecurityContext, domain.WaylandPassthrough},
			func() string { return frm.draft.Display.Wayland },
			func(val string) { frm.draft.Display.Wayland = val }),
		boolean("display.gpu",
			func() bool { return frm.draft.Display.GPU },
			func(val bool) { frm.draft.Display.GPU = val }),
		enum("network.mode", []string{domain.NetworkNone, domain.NetworkHost, domain.NetworkContainer},
			func() string { return frm.draft.Network.Mode },
			func(val string) { frm.draft.Network.Mode = val; frm.rebuildFields() }),
	)
	// network.target only applies to container mode — hide it otherwise (§6.4).
	if frm.draft.Network.Mode == domain.NetworkContainer {
		fields = append(fields, formField{label: "network.target", kind: kindText, input: &frm.target})
	}
	fields = append(fields,
		enum("theme.mode", []string{domain.ThemeHost, domain.ThemeNone},
			func() string { return frm.draft.Theme.Mode },
			func(val string) { frm.draft.Theme.Mode = val }),
		boolean("audio.pipewire",
			func() bool { return frm.draft.Audio.Pipewire },
			func(val bool) { frm.draft.Audio.Pipewire = val }),
		boolean("audio.legacy_alsa",
			func() bool { return frm.draft.Audio.LegacyALSA },
			func(val bool) { frm.draft.Audio.LegacyALSA = val }),
		boolean("terminal",
			func() bool { return frm.draft.App.Terminal },
			func(val bool) { frm.draft.App.Terminal = val }),
		boolean("multiterminal",
			func() bool { return frm.draft.App.Multiterminal },
			func(val bool) { frm.draft.App.Multiterminal = val }),
		boolean("keep_open",
			func() bool { return frm.draft.App.KeepOpen },
			func(val bool) { frm.draft.App.KeepOpen = val }),
		boolean("background",
			func() bool { return frm.draft.App.Background },
			func(val bool) { frm.draft.App.Background = val }),
		boolean("autostart",
			func() bool { return frm.draft.App.Autostart },
			func(val bool) { frm.draft.App.Autostart = val }),
		formField{label: "description", kind: kindText, input: &frm.desc},
		formField{label: "icon", kind: kindText, input: &frm.icon},
		formField{label: "advanced", kind: kindAction, info: frm.advancedSummary},
	)
	frm.fields = fields
}

// rebuildFields re-derives the visible field list (e.g. after a network.mode change
// shows or hides network.target) and keeps the cursor and focus valid.
func (frm *formModel) rebuildFields() {
	frm.buildFields()
	if frm.idx >= len(frm.fields) {
		frm.idx = len(frm.fields) - 1
	}
	frm.focus(frm.idx)
}

// reload swaps in a config edited out-of-band (the $EDITOR round-trip) and re-seeds
// the text inputs from it, keeping the same form (and its creating flag) so a
// half-finished new app isn't reset. Unlike newForm, it never re-templates.
func (frm *formModel) reload(cfg domain.AppConfig) {
	frm.draft = cfg
	if frm.draft.SchemaVersion == 0 {
		frm.draft.SchemaVersion = domain.SchemaVersion
	}
	frm.name.SetValue(cfg.App.Name)
	frm.image.SetValue(cfg.App.Image)
	frm.command.SetValue(strings.Join(cfg.App.Command, " "))
	frm.install.SetValue(cfg.App.Install)
	frm.desc.SetValue(cfg.App.Description)
	frm.icon.SetValue(cfg.App.Icon)
	frm.target.SetValue(cfg.Network.Target)
	frm.buildFields()
	frm.err = nil
	if frm.idx >= len(frm.fields) {
		frm.idx = len(frm.fields) - 1
	}
	frm.focus(frm.idx)
}

// applyPreset re-seeds the preset-controlled fields from a template (§4). Name,
// image, description, icon, and list-valued fields are left untouched, so picking a
// preset never silently discards what the user typed.
func (frm *formModel) applyPreset(preset string) {
	defaults, ok := domain.DefaultsFor(preset)
	if !ok {
		return
	}
	frm.draft.App.Preset = preset
	frm.draft.Display.Wayland = defaults.Display.Wayland
	frm.draft.Display.GPU = defaults.Display.GPU
	frm.draft.Network.Mode = defaults.Network.Mode
	frm.draft.Network.BlockDNS = defaults.Network.BlockDNS
	frm.draft.Audio = defaults.Audio
	frm.draft.Theme = defaults.Theme
	frm.rebuildFields() // a preset can change network.mode → target visibility
}

func (frm *formModel) advancedSummary() string {
	return fmt.Sprintf("cmd=%d  mounts=%d  ssh=%d gpg=%d  caps=%d  ipv4=%d ipv6=%d ports=%d  depends_on=%d   (enter: edit TOML in $EDITOR)",
		len(frm.draft.App.Command), len(frm.draft.Mounts), len(frm.draft.Keys.SSH), len(frm.draft.Keys.GPG), len(frm.draft.Capabilities.Extra),
		len(frm.draft.Network.IPv4CIDR), len(frm.draft.Network.IPv6CIDR), len(frm.draft.Network.Ports),
		len(frm.draft.DependsOn.Containers))
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

	// Field-kind-independent commands first. These are control keys (esc, ctrl+*,
	// tab), so they never collide with typing into a focused text field.
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

	// Everything else depends on the focused field's kind. The scheme decides which
	// keys fire each gesture; the dispatch by kind stays here.
	fld := frm.fields[frm.idx]
	switch fld.kind {
	case kindText:
		var cmd tea.Cmd
		*fld.input, cmd = fld.input.Update(msg)
		return cmd, formStay
	case kindMultiline:
		// Enter on an EMPTY field applies the suggested install prefix (InstallHint
		// without its <pkg> placeholder), so the user types only the package; once the
		// field has content, enter inserts a newline for a multi-step script (the lines
		// are joined into one RUN at build — domain.installScript, §9.1).
		if scheme.Is(keys.CtxForm, keys.Activate, keyStr) && strings.TrimSpace(fld.area.Value()) == "" {
			fld.area.SetValue(installTemplate(frm.image.Value()))
			return nil, formStay
		}
		var cmd tea.Cmd
		*fld.area, cmd = fld.area.Update(msg)
		return cmd, formStay
	case kindAction:
		if scheme.Is(keys.CtxForm, keys.Activate, keyStr) {
			return nil, formEdit
		}
	case kindEnum:
		switch {
		case scheme.Is(keys.CtxForm, keys.EnumNext, keyStr):
			fld.set(cycle(fld.options, fld.get(), +1))
		case scheme.Is(keys.CtxForm, keys.EnumPrev, keyStr):
			fld.set(cycle(fld.options, fld.get(), -1))
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
	for _, input := range []*textinput.Model{&frm.name, &frm.image, &frm.command, &frm.desc, &frm.icon, &frm.target} {
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
func (frm *formModel) toConfig() domain.AppConfig {
	cfg := frm.draft
	if frm.creating {
		cfg.App.Name = strings.TrimSpace(frm.name.Value())
	}
	cfg.App.Image = strings.TrimSpace(frm.image.Value())
	// Entrypoint: the field is a space-separated convenience view of App.Command.
	// Re-split only when it actually changed, so a complex argv authored via the
	// advanced TOML editor (quoted, multi-word args) isn't flattened on a plain save.
	if val := frm.command.Value(); strings.TrimSpace(val) != strings.TrimSpace(strings.Join(frm.draft.App.Command, " ")) {
		cfg.App.Command = splitCommand(val)
	}
	cfg.App.Install = strings.TrimSpace(frm.install.Value())
	cfg.App.Description = frm.desc.Value()
	cfg.App.Icon = strings.TrimSpace(frm.icon.Value())
	// network.target only applies to container mode; clear any stale value left from a
	// mode switch so it can't trip launch-time validation.
	if cfg.Network.Mode == domain.NetworkContainer {
		cfg.Network.Target = strings.TrimSpace(frm.target.Value())
	} else {
		cfg.Network.Target = ""
	}
	cfg.SchemaVersion = domain.SchemaVersion
	return cfg
}

// splitCommand turns the entrypoint field's text into an argv on whitespace,
// returning nil (not an empty slice) when blank so a cleared command marshals away
// and "image default" is preserved. Whitespace-splitting is deliberately simple — a
// command needing quoting stays editable as full argv via the advanced TOML row.
func splitCommand(text string) []string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// installTemplate is InstallHint(image) with its trailing <pkg> placeholder removed
// and a single trailing space — the prefix the install field's apply gesture (enter
// on an empty field) inserts, so the user lands ready to type a package name.
func installTemplate(image string) string {
	hint := domain.InstallHint(strings.TrimSpace(image))
	return strings.TrimRight(strings.TrimSuffix(hint, "<pkg>"), " ") + " "
}

func cycle(opts []string, cur string, dir int) string {
	idx := 0
	for index, opt := range opts {
		if opt == cur {
			idx = index
			break
		}
	}
	idx = (idx + dir + len(opts)) % len(opts)
	return opts[idx]
}

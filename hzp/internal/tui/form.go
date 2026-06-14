package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
)

// formResult is what a form key-press tells the parent to do.
type formResult int

const (
	formStay formResult = iota
	formSave
	formCancel
)

type fieldKind int

const (
	kindText fieldKind = iota // free text via a textinput
	kindEnum                  // cycle through a fixed option set
	kindBool                  // toggle
	kindInfo                  // read-only (shown, not navigable)
)

// formField is one editable (or informational) row. Closures read/write the
// form's draft config so the rendered value is always the actual value (§4).
type formField struct {
	label   string
	kind    fieldKind
	input   *textinput.Model
	options []string
	get     func() string
	set     func(string)
	bget    func() bool
	bset    func(bool)
	info    func() string
}

// formModel is the create/edit form. draft holds the enum/bool values directly;
// the text fields live in their own inputs and are folded into draft on save.
type formModel struct {
	creating bool
	draft    config.AppConfig

	name   textinput.Model
	image  textinput.Model
	desc   textinput.Model
	icon   textinput.Model
	target textinput.Model

	fields []formField
	idx    int
	err    error
}

func newForm(base config.AppConfig, creating bool) *formModel {
	if creating {
		base, _ = config.DefaultsFor(config.PresetStrict)
	}
	f := &formModel{creating: creating, draft: base}
	if f.draft.SchemaVersion == 0 {
		f.draft.SchemaVersion = config.SchemaVersion
	}
	if f.draft.App.Preset == "" {
		f.draft.App.Preset = config.PresetStrict // label only; never enforced (§4)
	}

	f.name = newInput(f.draft.App.Name, "firefox")
	f.image = newInput(f.draft.App.Image, "docker.io/…@sha256:… (trusted-* may use a tag)")
	f.desc = newInput(f.draft.App.Description, "")
	f.icon = newInput(f.draft.App.Icon, "")
	f.target = newInput(f.draft.Network.Target, "vpn-container")

	f.buildFields()
	f.idx = -1
	f.focusNext() // land on the first editable field
	return f
}

func newInput(value, placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = placeholder
	ti.CharLimit = 256
	ti.SetValue(value)
	return ti
}

func (f *formModel) buildFields() {
	enum := func(label string, opts []string, get func() string, set func(string)) formField {
		return formField{label: label, kind: kindEnum, options: opts, get: get, set: set}
	}
	boolean := func(label string, get func() bool, set func(bool)) formField {
		return formField{label: label, kind: kindBool, bget: get, bset: set}
	}

	var fields []formField
	if f.creating {
		fields = append(fields, formField{label: "name", kind: kindText, input: &f.name})
	} else {
		fields = append(fields, formField{label: "name", kind: kindInfo,
			info: func() string { return f.draft.App.Name + "   (rename = delete + recreate)" }})
	}
	fields = append(fields,
		formField{label: "image", kind: kindText, input: &f.image},
		enum("preset", []string{config.PresetStrict, config.PresetStandard, config.PresetNetworked},
			func() string { return f.draft.App.Preset },
			func(v string) { f.applyPreset(v) }),
		enum("display.wayland", []string{config.WaylandSecurityContext, config.WaylandPassthrough},
			func() string { return f.draft.Display.Wayland },
			func(v string) { f.draft.Display.Wayland = v }),
		boolean("display.gpu",
			func() bool { return f.draft.Display.GPU },
			func(v bool) { f.draft.Display.GPU = v }),
		enum("network.mode", []string{config.NetworkNone, config.NetworkPasta, config.NetworkContainer},
			func() string { return f.draft.Network.Mode },
			func(v string) { f.draft.Network.Mode = v }),
		formField{label: "network.target", kind: kindText, input: &f.target},
		enum("theme.mode", []string{config.ThemeHost, config.ThemeNone},
			func() string { return f.draft.Theme.Mode },
			func(v string) { f.draft.Theme.Mode = v }),
		boolean("audio.pipewire",
			func() bool { return f.draft.Audio.Pipewire },
			func(v bool) { f.draft.Audio.Pipewire = v }),
		boolean("audio.legacy_alsa",
			func() bool { return f.draft.Audio.LegacyALSA },
			func(v bool) { f.draft.Audio.LegacyALSA = v }),
		boolean("background",
			func() bool { return f.draft.App.Background },
			func(v bool) { f.draft.App.Background = v }),
		boolean("autostart",
			func() bool { return f.draft.App.Autostart },
			func(v bool) { f.draft.App.Autostart = v }),
		formField{label: "description", kind: kindText, input: &f.desc},
		formField{label: "icon", kind: kindText, input: &f.icon},
		formField{label: "advanced", kind: kindInfo, info: f.advancedSummary},
	)
	f.fields = fields
}

// applyPreset re-seeds the preset-controlled fields from a template (§4). Name,
// image, description, icon, and list-valued fields are left untouched, so picking
// a preset never silently discards what the user typed.
func (f *formModel) applyPreset(p string) {
	d, ok := config.DefaultsFor(p)
	if !ok {
		return
	}
	f.draft.App.Preset = p
	f.draft.Display.Wayland = d.Display.Wayland
	f.draft.Display.GPU = d.Display.GPU
	f.draft.Network.Mode = d.Network.Mode
	f.draft.Network.BlockDNS = d.Network.BlockDNS
	f.draft.Audio = d.Audio
	f.draft.Theme = d.Theme
}

func (f *formModel) advancedSummary() string {
	return fmt.Sprintf("mounts=%d  ssh=%d gpg=%d  caps=%d  ipv4=%d ipv6=%d ports=%d  depends_on=%d   (edit TOML)",
		len(f.draft.Mounts), len(f.draft.Keys.SSH), len(f.draft.Keys.GPG), len(f.draft.Capabilities.Extra),
		len(f.draft.Network.IPv4CIDR), len(f.draft.Network.IPv6CIDR), len(f.draft.Network.Ports),
		len(f.draft.DependsOn.Containers))
}

func (f *formModel) update(msg tea.Msg) (tea.Cmd, formResult) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, formStay
	}
	switch key.String() {
	case "esc":
		return nil, formCancel
	case "ctrl+s":
		return nil, formSave
	case "tab", "down":
		f.focusNext()
		return nil, formStay
	case "shift+tab", "up":
		f.focusPrev()
		return nil, formStay
	}

	fld := f.fields[f.idx]
	switch fld.kind {
	case kindText:
		var cmd tea.Cmd
		*fld.input, cmd = fld.input.Update(msg)
		return cmd, formStay
	case kindEnum:
		switch key.String() {
		case "right", "l", " ":
			fld.set(cycle(fld.options, fld.get(), +1))
		case "left", "h":
			fld.set(cycle(fld.options, fld.get(), -1))
		}
	case kindBool:
		switch key.String() {
		case " ", "enter", "left", "right", "h", "l":
			fld.bset(!fld.bget())
		}
	}
	return nil, formStay
}

func (f *formModel) focus(i int) {
	f.idx = i
	for _, in := range []*textinput.Model{&f.name, &f.image, &f.desc, &f.icon, &f.target} {
		in.Blur()
	}
	if fld := f.fields[i]; fld.kind == kindText && fld.input != nil {
		fld.input.Focus()
	}
}

func (f *formModel) focusNext() {
	for n := 0; n < len(f.fields); n++ {
		i := (f.idx + 1 + n) % len(f.fields)
		if f.fields[i].kind != kindInfo {
			f.focus(i)
			return
		}
	}
}

func (f *formModel) focusPrev() {
	for n := 0; n < len(f.fields); n++ {
		i := ((f.idx-1-n)%len(f.fields) + len(f.fields)) % len(f.fields)
		if f.fields[i].kind != kindInfo {
			f.focus(i)
			return
		}
	}
}

// toConfig folds the text inputs into the draft and returns the config to save.
func (f *formModel) toConfig() config.AppConfig {
	c := f.draft
	if f.creating {
		c.App.Name = strings.TrimSpace(f.name.Value())
	}
	c.App.Image = strings.TrimSpace(f.image.Value())
	c.App.Description = f.desc.Value()
	c.App.Icon = strings.TrimSpace(f.icon.Value())
	c.Network.Target = strings.TrimSpace(f.target.Value())
	c.SchemaVersion = config.SchemaVersion
	return c
}

func cycle(opts []string, cur string, dir int) string {
	idx := 0
	for i, o := range opts {
		if o == cur {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(opts)) % len(opts)
	return opts[idx]
}

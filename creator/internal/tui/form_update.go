package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/creator/internal/keys"
)

func (frm *formModel) update(msg tea.Msg) (tea.Cmd, formResult) {
	message, ok := msg.(tea.KeyMsg)
	if !ok || frm.idx < 0 || frm.idx >= len(frm.fields) {
		return nil, formStay
	}
	key, scheme := message.String(), frm.scheme
	field := frm.fields[frm.idx]
	if field.kind == kindMultiline && (key == "up" || key == "down") {
		var command tea.Cmd
		*field.area, command = field.area.Update(msg)
		return command, formStay
	}
	switch {
	case scheme.Is(keys.CtxForm, keys.Cancel, key):
		return nil, formCancel
	case scheme.Is(keys.CtxForm, keys.Save, key):
		return nil, formSave
	case scheme.Is(keys.CtxForm, keys.NextField, key):
		frm.focusNext()
		return nil, formStay
	case scheme.Is(keys.CtxForm, keys.PrevField, key):
		frm.focusPrev()
		return nil, formStay
	case scheme.Is(keys.CtxForm, keys.ClearField, key):
		if field.input != nil {
			field.input.SetValue("")
		}
		if field.area != nil {
			field.area.SetValue("")
		}
		return nil, formStay
	case scheme.Is(keys.CtxForm, keys.ResolveImage, key):
		if field.label == "image" && frm.draft.Type == "ZincContainer" {
			return nil, formResolve
		}
		return nil, formStay
	}
	switch field.kind {
	case kindText:
		var command tea.Cmd
		*field.input, command = field.input.Update(msg)
		return command, formStay
	case kindMultiline:
		var command tea.Cmd
		*field.area, command = field.area.Update(msg)
		return command, formStay
	case kindAction:
		if scheme.Is(keys.CtxForm, keys.Activate, key) {
			return nil, formEdit
		}
	case kindBool:
		if scheme.Is(keys.CtxForm, keys.Toggle, key) {
			field.bset(!field.bget())
		}
	case kindEnum:
		if scheme.Is(keys.CtxForm, keys.Toggle, key) {
			field.set(nextValue(field.values, field.get()))
			if field.rebuild {
				frm.buildFields()
				frm.focus(frm.idx)
			}
		}
	}
	return nil, formStay
}

func nextValue(values []string, current string) string {
	for index, value := range values {
		if value == current {
			return values[(index+1)%len(values)]
		}
	}
	return values[0]
}

func (frm *formModel) focus(index int) {
	for _, field := range append(append([]formField(nil), frm.common...), frm.virtual...) {
		if field.input != nil {
			field.input.Blur()
		}
		if field.area != nil {
			field.area.Blur()
		}
	}
	frm.idx = index
	field := frm.fields[index]
	if field.input != nil {
		field.input.Focus()
	}
	if field.area != nil {
		field.area.Focus()
	}
}

func (frm *formModel) focusNext() { frm.moveFocus(1) }
func (frm *formModel) focusPrev() { frm.moveFocus(-1) }

func (frm *formModel) moveFocus(direction int) {
	for step := 1; step <= len(frm.fields); step++ {
		index := (frm.idx + direction*step + len(frm.fields)) % len(frm.fields)
		if frm.fields[index].kind != kindInfo {
			frm.focus(index)
			return
		}
	}
}

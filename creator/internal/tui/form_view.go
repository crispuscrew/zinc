package tui

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/creator/internal/keys"
)

func (frm *formModel) view() string {
	title := "New app"
	if !frm.creating {
		title = "Edit " + frm.draft.AppNameID
	}
	var lines []string
	focusLine := 0
	for index, field := range frm.fields {
		if index == frm.idx {
			focusLine = len(lines)
		}
		lines = append(lines, strings.Split(renderField(field, index == frm.idx), "\n")...)
	}
	height := frm.height
	if height <= 0 {
		height = 24
	}
	available := max(height-8, 5)
	start := max(0, focusLine-available/3)
	end := min(len(lines), start+available)
	output := titleStyle.Render(title) + fmt.Sprintf(" (%d/%d)\n\n", frm.idx+1, len(frm.fields))
	output += strings.Join(lines[start:end], "\n") + "\n"
	if frm.err != nil {
		output += errStyle.Render(oneLine(frm.err.Error(), 512)) + "\n"
	}
	if frm.rawFlagsPresent() {
		output += errStyle.Render("WARNING: raw backend argv may override Zinc isolation/network/device/lifecycle controls.") + "\n"
	}
	return output + frm.footer()
}

func renderField(field formField, focused bool) string {
	cursor := "  "
	if focused {
		cursor = "> "
	}
	label := fmt.Sprintf("%-34s", field.label)
	if focused {
		label = selected.Render(label)
	}
	if field.kind == kindMultiline {
		return cursor + label + "\n    " + strings.ReplaceAll(field.area.View(), "\n", "\n    ")
	}
	var value string
	switch field.kind {
	case kindText:
		value = field.input.View()
	case kindBool:
		value = renderBool(field.bget())
	case kindEnum:
		value = field.get()
		if value == "" {
			value = "(automatic)"
		}
	case kindInfo, kindAction:
		value = dim.Render(field.info())
	}
	return cursor + label + value
}

func (frm *formModel) footer() string {
	var hints []string
	add := func(action keys.Action, label string) {
		if hint := frm.scheme.HintPrimary(keys.CtxForm, action); hint != "" {
			hints = append(hints, hint+" "+label)
		}
	}
	add(keys.NextField, "move")
	if frm.idx >= 0 && frm.idx < len(frm.fields) {
		field := frm.fields[frm.idx]
		switch field.kind {
		case kindText:
			add(keys.ClearField, "clear")
			if field.label == "image" && frm.draft.Type == "ZincContainer" {
				add(keys.ResolveImage, "resolve")
			}
		case kindMultiline:
			add(keys.ClearField, "clear")
			add(keys.Activate, "newline")
		case kindBool:
			add(keys.Toggle, "toggle")
		case kindEnum:
			add(keys.Toggle, "change")
		case kindAction:
			add(keys.Activate, "edit YAML")
		}
	}
	add(keys.Save, "save")
	add(keys.Cancel, "cancel")
	return help.Render(strings.Join(hints, " | "))
}

func renderBool(value bool) string {
	if value {
		return enumSel.Render("[x] on")
	}
	return dim.Render("[ ] off")
}

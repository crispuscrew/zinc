package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/crispuscrew/hyprzinc/hzc/internal/keys"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	help       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	dim        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	selected   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	enumSel    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	runDot     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	offDot     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func (mdl Model) View() string {
	if mdl.quitting {
		return ""
	}
	switch mdl.mode {
	case modeForm:
		return mdl.form.view()
	case modeLogs:
		return mdl.logsView()
	case modeConfirmDelete:
		return mdl.confirmView()
	case modeKeys:
		return mdl.keysView()
	default:
		return mdl.listView()
	}
}

func (mdl Model) listView() string {
	var bld strings.Builder
	bld.WriteString(titleStyle.Render("HyprZinc — apps") + "\n\n")

	if len(mdl.apps) == 0 {
		bld.WriteString(dim.Render("no apps yet — press n to create one") + "\n")
	}
	for idx, row := range mdl.apps {
		cursor := "  "
		if idx == mdl.cursor {
			cursor = "▸ "
		}
		dot := offDot.Render("○")
		if row.running {
			dot = runDot.Render("●")
		}
		name := fmt.Sprintf("%-16s", row.cfg.App.Name)
		preset := fmt.Sprintf("%-10s", presetLabel(row.cfg.App.Preset))
		detail := row.cfg.App.Image
		if row.loadErr != nil {
			detail = errStyle.Render("(invalid: " + row.loadErr.Error() + ")")
		}
		line := cursor + dot + " " + name + " " + preset + " " + detail
		if idx == mdl.cursor {
			line = selected.Render(line)
		}
		bld.WriteString(line + "\n")
	}

	bld.WriteString("\n")
	switch {
	case mdl.err != nil:
		bld.WriteString(errStyle.Render("✗ "+mdl.err.Error()) + "\n")
	case mdl.status != "":
		bld.WriteString(dim.Render(mdl.status) + "\n")
	}
	bld.WriteString(mdl.listFooter())
	return bld.String()
}

// listFooter shows only the actions that apply to the current selection and its
// state — so the help line is never a "porridge" of every key (§9.1). Global actions
// (new/refresh/keys/quit) always show; edit/delete need a selection; run shows when
// stopped (or for any multiterminal app, where it adds a terminal), stop/logs when
// running, build when an install line is set. Each gesture shows its primary key only.
func (mdl Model) listFooter() string {
	scheme := mdl.keys.Scheme
	var segs []string
	add := func(act keys.Action, label string) {
		if hint := scheme.HintPrimary(keys.CtxList, act); hint != "" {
			segs = append(segs, hint+" "+label)
		}
	}
	add(keys.New, "new")
	if row, ok := mdl.selected(); ok {
		if row.loadErr == nil {
			add(keys.Edit, "edit")
			if !row.running || row.cfg.App.Multiterminal {
				add(keys.Run, "run")
			}
			if row.cfg.App.Multiterminal {
				add(keys.Shell, "shell")
			}
			if row.running {
				add(keys.Stop, "stop")
				add(keys.Logs, "logs")
			}
			if row.cfg.App.Install != "" {
				add(keys.Build, "build")
			}
		}
		add(keys.Delete, "delete") // a broken (loadErr) app can still be removed
	}
	add(keys.Refresh, "refresh")
	add(keys.Keys, "keys")
	add(keys.Quit, "quit")
	return help.Render(strings.Join(segs, " · "))
}

func (frm *formModel) view() string {
	var bld strings.Builder
	title := "New app"
	if !frm.creating {
		title = "Edit " + frm.draft.App.Name
	}
	bld.WriteString(titleStyle.Render(title) + "\n\n")

	for idx, fld := range frm.fields {
		cursor := "  "
		if idx == frm.idx {
			cursor = "▸ "
		}
		label := fmt.Sprintf("%-18s", fld.label)
		if idx == frm.idx {
			label = selected.Render(label)
		}

		// A multi-line field renders its label on the cursor line, then the textarea
		// block beneath it (indented), since its View spans several lines.
		if fld.kind == kindMultiline {
			bld.WriteString(cursor + label + "\n")
			for _, line := range strings.Split(fld.area.View(), "\n") {
				bld.WriteString("    " + line + "\n")
			}
			continue
		}

		var val string
		switch fld.kind {
		case kindText:
			val = fld.input.View()
			if idx != frm.idx && fld.input.Value() == "" {
				val = dim.Render("(empty)")
			}
		case kindEnum:
			val = renderEnum(fld.options, fld.get(), idx == frm.idx)
		case kindBool:
			val = renderBool(fld.bget())
		case kindInfo, kindAction:
			val = dim.Render(fld.info())
		}
		bld.WriteString(cursor + label + val + "\n")
	}

	if frm.err != nil {
		bld.WriteString("\n" + errStyle.Render("✗ "+frm.err.Error()) + "\n")
	}
	bld.WriteString("\n" + frm.footer())
	return bld.String()
}

// footer shows only the gestures for the focused field's kind, plus the always-
// available move/save/cancel — so an enum row doesn't advertise "toggle", a bool row
// doesn't advertise "resolve", and the help line stays short (§9.1). Each gesture
// shows its primary key only, drawn from the active scheme.
func (frm *formModel) footer() string {
	scheme := frm.scheme
	var segs []string
	add := func(hint, label string) {
		if hint != "" {
			segs = append(segs, hint+" "+label)
		}
	}
	add(scheme.HintPrimary(keys.CtxForm, keys.NextField), "move")
	if frm.idx >= 0 && frm.idx < len(frm.fields) {
		switch fld := frm.fields[frm.idx]; fld.kind {
		case kindText:
			add(scheme.HintPrimary(keys.CtxForm, keys.ClearField), "clear")
			if fld.label == "image" {
				add(scheme.HintPrimary(keys.CtxForm, keys.ResolveImage), "resolve")
			}
		case kindMultiline:
			add(scheme.HintPrimary(keys.CtxForm, keys.ClearField), "clear")
			if fld.area != nil && strings.TrimSpace(fld.area.Value()) == "" {
				add(scheme.HintPrimary(keys.CtxForm, keys.Activate), "apply hint")
			} else {
				add(scheme.HintPrimary(keys.CtxForm, keys.Activate), "newline")
			}
		case kindEnum:
			change := strings.Trim(scheme.HintPrimary(keys.CtxForm, keys.EnumPrev)+"/"+scheme.HintPrimary(keys.CtxForm, keys.EnumNext), "/")
			add(change, "change")
		case kindBool:
			add(scheme.HintPrimary(keys.CtxForm, keys.Toggle), "toggle")
		case kindAction:
			add(scheme.HintPrimary(keys.CtxForm, keys.Activate), "edit")
		}
	}
	add(scheme.HintPrimary(keys.CtxForm, keys.Save), "save")
	add(scheme.HintPrimary(keys.CtxForm, keys.Cancel), "cancel")
	return help.Render(strings.Join(segs, " · "))
}

func (mdl Model) logsView() string {
	header := titleStyle.Render("logs — " + mdl.logsName)
	// Scrolling is the viewport's own built-in (not a scheme action), so it stays
	// literal; only "back" is scheme-driven.
	footer := help.Render(fmt.Sprintf("↑/↓/pgup/pgdn scroll · %s back", mdl.keys.Scheme.HintPrimary(keys.CtxLogs, keys.Back)))
	return header + "\n" + mdl.logs.View() + "\n" + footer
}

func (mdl Model) confirmView() string {
	scheme := mdl.keys.Scheme
	return "\n  " + titleStyle.Render("Delete "+mdl.confirmName+"?") +
		"\n\n  " + help.Render(fmt.Sprintf("%s confirm · %s cancel",
		scheme.HintPrimary(keys.CtxConfirm, keys.Yes), scheme.HintPrimary(keys.CtxConfirm, keys.No))) + "\n"
}

// keysView is the keybind-scheme picker (modeKeys): every selectable scheme, the
// active one marked, built-in vs custom labelled.
func (mdl Model) keysView() string {
	var bld strings.Builder
	bld.WriteString(titleStyle.Render("HyprZinc — keybind schemes") + "\n\n")
	if len(mdl.keysList) == 0 {
		bld.WriteString(dim.Render("loading…") + "\n")
	}
	for idx, name := range mdl.keysList {
		cursor := "  "
		if idx == mdl.keysCursor {
			cursor = "▸ "
		}
		kind := "custom"
		if keys.IsBuiltin(name) {
			kind = "built-in"
		}
		mark := " "
		if name == mdl.keys.Name {
			mark = "●"
		}
		label := fmt.Sprintf("%s %-18s", mark, name)
		if idx == mdl.keysCursor {
			label = selected.Render(label)
		}
		bld.WriteString(cursor + label + " " + dim.Render("("+kind+")") + "\n")
	}
	bld.WriteString("\n")
	if mdl.err != nil {
		bld.WriteString(errStyle.Render("✗ "+mdl.err.Error()) + "\n")
	}
	// The picker reuses the list scheme's up/down to move (so vim users get j/k here
	// too); apply/edit/back are intrinsic picker keys, so they stay literal.
	scheme := mdl.keys.Scheme
	bld.WriteString(help.Render(fmt.Sprintf("%s/%s move · enter apply · e edit/new custom · esc back",
		scheme.HintPrimary(keys.CtxList, keys.Up), scheme.HintPrimary(keys.CtxList, keys.Down))))
	return bld.String()
}

func renderEnum(opts []string, cur string, active bool) string {
	parts := make([]string, len(opts))
	for idx, opt := range opts {
		if opt == cur {
			parts[idx] = enumSel.Render(opt)
		} else {
			parts[idx] = dim.Render(opt)
		}
	}
	rendered := strings.Join(parts, "  ")
	if active {
		rendered = "‹ " + rendered + " ›"
	}
	return rendered
}

func renderBool(val bool) string {
	if val {
		return enumSel.Render("[x] on")
	}
	return dim.Render("[ ] off")
}

func presetLabel(preset string) string {
	if preset == "" {
		return "(none)"
	}
	return preset
}

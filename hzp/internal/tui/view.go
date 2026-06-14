package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	switch m.mode {
	case modeForm:
		return m.form.view()
	case modeLogs:
		return m.logsView()
	case modeConfirmDelete:
		return m.confirmView()
	default:
		return m.listView()
	}
}

func (m Model) listView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("HyprZinc — apps") + "\n\n")

	if len(m.apps) == 0 {
		b.WriteString(dim.Render("no apps yet — press n to create one") + "\n")
	}
	for i, row := range m.apps {
		cursor := "  "
		if i == m.cursor {
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
		if i == m.cursor {
			line = selected.Render(line)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")
	switch {
	case m.err != nil:
		b.WriteString(errStyle.Render("✗ "+m.err.Error()) + "\n")
	case m.status != "":
		b.WriteString(dim.Render(m.status) + "\n")
	}
	b.WriteString(help.Render("n new · e edit · r run · s stop · l logs · d delete · g refresh · q quit"))
	return b.String()
}

func (f *formModel) view() string {
	var b strings.Builder
	title := "New app"
	if !f.creating {
		title = "Edit " + f.draft.App.Name
	}
	b.WriteString(titleStyle.Render(title) + "\n\n")

	for i, fld := range f.fields {
		cursor := "  "
		if i == f.idx {
			cursor = "▸ "
		}
		label := fmt.Sprintf("%-18s", fld.label)
		if i == f.idx {
			label = selected.Render(label)
		}

		var val string
		switch fld.kind {
		case kindText:
			val = fld.input.View()
			if i != f.idx && fld.input.Value() == "" {
				val = dim.Render("(empty)")
			}
		case kindEnum:
			val = renderEnum(fld.options, fld.get(), i == f.idx)
		case kindBool:
			val = renderBool(fld.bget())
		case kindInfo:
			val = dim.Render(fld.info())
		}
		b.WriteString(cursor + label + val + "\n")
	}

	if f.err != nil {
		b.WriteString("\n" + errStyle.Render("✗ "+f.err.Error()) + "\n")
	}
	b.WriteString("\n" + help.Render("tab/↑↓ move · ←/→/space change · ctrl+s save · esc cancel"))
	return b.String()
}

func (m Model) logsView() string {
	header := titleStyle.Render("logs — " + m.logsName)
	footer := help.Render("↑/↓/pgup/pgdn scroll · esc back")
	return header + "\n" + m.logs.View() + "\n" + footer
}

func (m Model) confirmView() string {
	return "\n  " + titleStyle.Render("Delete "+m.confirmName+"?") +
		"\n\n  " + help.Render("y confirm · n cancel") + "\n"
}

func renderEnum(opts []string, cur string, active bool) string {
	parts := make([]string, len(opts))
	for i, o := range opts {
		if o == cur {
			parts[i] = enumSel.Render(o)
		} else {
			parts[i] = dim.Render(o)
		}
	}
	s := strings.Join(parts, "  ")
	if active {
		s = "‹ " + s + " ›"
	}
	return s
}

func renderBool(v bool) string {
	if v {
		return enumSel.Render("[x] on")
	}
	return dim.Render("[ ] off")
}

func presetLabel(p string) string {
	if p == "" {
		return "(none)"
	}
	return p
}

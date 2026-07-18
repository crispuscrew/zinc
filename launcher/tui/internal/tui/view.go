package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	promptStyle   = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	runningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green dot
	descStyle     = lipgloss.NewStyle().Faint(true)
	hintStyle     = lipgloss.NewStyle().Faint(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// View renders the picker: a prompt, the filtered list (a running dot, the name, a dim
// description), and a footer of hints or the current status.
func (mdl Model) View() string {
	if len(mdl.apps) == 0 {
		return promptStyle.Render("Zinc launcher") + "\n\n" +
			hintStyle.Render("no apps defined yet - create one with: zcc new <name> --image <img>") + "\n\n" +
			hintStyle.Render("esc quit") + "\n"
	}

	var bld strings.Builder
	bld.WriteString(promptStyle.Render("> ") + mdl.query + "█\n\n") // block cursor

	if len(mdl.filtered) == 0 {
		bld.WriteString(hintStyle.Render("no matches") + "\n")
	} else {
		start, end := mdl.window()
		for pos := start; pos < end; pos++ {
			bld.WriteString(mdl.renderRow(pos))
		}
	}

	bld.WriteString("\n")
	if mdl.status != "" {
		style := hintStyle
		if strings.Contains(mdl.status, ": ") { // a launch error carries "name: message"
			style = errorStyle
		}
		bld.WriteString(style.Render(mdl.status) + "\n")
	} else {
		bld.WriteString(hintStyle.Render(fmt.Sprintf(
			"enter launch · up/down move · type to filter · esc quit  (%d/%d)",
			len(mdl.filtered), len(mdl.apps))) + "\n")
	}
	return bld.String()
}

// renderRow renders one filtered entry at position pos (an index into mdl.filtered).
func (mdl Model) renderRow(pos int) string {
	app := mdl.apps[mdl.filtered[pos].Index]

	dot := "  "
	if app.Running {
		dot = runningStyle.Render("●") + " " // filled circle
	}

	name := app.Name
	pointer := "  "
	if pos == mdl.cursor {
		pointer = selectedStyle.Render("▸ ") // right-pointing triangle
		name = selectedStyle.Render(name)
	}

	line := pointer + dot + name
	if desc := strings.TrimSpace(app.Description); desc != "" {
		line += "  " + descStyle.Render(desc)
	}
	return line + "\n"
}

// window returns the [start,end) slice of mdl.filtered to display, scrolled to keep the
// cursor visible within the available height.
func (mdl Model) window() (int, int) {
	rows := 15
	if mdl.height > 6 {
		rows = mdl.height - 5 // prompt + blank + footer + margin
	}
	if rows < 1 {
		rows = 1
	}
	if len(mdl.filtered) <= rows {
		return 0, len(mdl.filtered)
	}
	start := 0
	if mdl.cursor >= rows {
		start = mdl.cursor - rows + 1
	}
	end := start + rows
	if end > len(mdl.filtered) {
		end = len(mdl.filtered)
		start = end - rows
	}
	return start, end
}

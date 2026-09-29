package tui

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/creator/internal/keys"
)

func (mdl Model) logsView() string {
	return titleStyle.Render("logs - "+mdl.logsName) + "\n" + mdl.logs.View() + "\n" + help.Render("up/down/pgup/pgdn scroll | "+mdl.keys.Scheme.HintPrimary(keys.CtxLogs, keys.Back)+" back")
}

func (mdl Model) confirmView() string {
	return titleStyle.Render("Delete "+mdl.confirmName+"?") + "\n" + help.Render(mdl.keys.Scheme.HintPrimary(keys.CtxConfirm, keys.Yes)+" confirm | "+mdl.keys.Scheme.HintPrimary(keys.CtxConfirm, keys.No)+" cancel")
}

func (mdl Model) renameView() string {
	return titleStyle.Render("Rename "+mdl.renameFrom) + "\n\n" + mdl.rename.View() + "\n" + help.Render("enter rename | esc cancel; stop the app first")
}

func (mdl Model) keysView() string {
	var output strings.Builder
	output.WriteString(titleStyle.Render("Zinc - keybind schemes") + "\n\n")
	if len(mdl.keysList) == 0 {
		output.WriteString("loading...\n")
	}
	for index, name := range mdl.keysList {
		cursor, mark, kind := "  ", " ", "custom"
		if index == mdl.keysCursor {
			cursor = "> "
		}
		if name == mdl.keys.Name {
			mark = "*"
		}
		if keys.IsBuiltin(name) {
			kind = "built-in"
		}
		output.WriteString(fmt.Sprintf("%s%s %-18s (%s)\n", cursor, mark, name, kind))
	}
	if mdl.err != nil {
		output.WriteString(errStyle.Render(oneLine(mdl.err.Error(), 512)) + "\n")
	}
	output.WriteString(help.Render("up/down move | enter apply | e edit | esc back"))
	return output.String()
}

// All list text is untrusted file/runtime output; remove cursor controls and
// bidirectional overrides before placing it on a terminal row.
func oneLine(text string, limit int) string {
	var output strings.Builder
	kept := 0
	for _, char := range text {
		if char < 0x20 || char == 0x7f || (char >= 0x80 && char <= 0x9f) {
			continue
		}
		if char == '\u202e' || char == '\u202d' || char == '\u2028' || char == '\u2029' {
			continue
		}
		if kept >= limit {
			output.WriteString("...")
			break
		}
		output.WriteRune(char)
		kept++
	}
	return output.String()
}

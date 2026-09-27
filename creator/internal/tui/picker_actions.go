package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/creator/internal/keys"
	"strings"
)

func (mdl Model) handleKeysKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "q":
		mdl.mode = modeList
		return mdl, nil
	case "enter":
		if len(mdl.keysList) > 0 {
			return mdl, setScheme(mdl.keysList[mdl.keysCursor])
		}
	case "e":
		if len(mdl.keysList) > 0 {
			return mdl, editScheme(mdl.keysList[mdl.keysCursor])
		}
	}
	switch action, _ := mdl.keys.Scheme.Resolve(keys.CtxList, message.String()); action {
	case keys.Up:
		mdl.keysCursor = max(mdl.keysCursor-1, 0)
	case keys.Down:
		mdl.keysCursor = min(mdl.keysCursor+1, max(len(mdl.keysList)-1, 0))
	}
	return mdl, nil
}

func (mdl Model) handleRenameKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		mdl.mode = modeList
		return mdl, nil
	case "enter":
		from, target := mdl.renameFrom, strings.TrimSpace(mdl.rename.Value())
		mdl.mode = modeList
		if target == "" || target == from {
			return mdl, nil
		}
		mdl.status = "renaming " + from + " -> " + target
		return mdl, renameApp(mdl.svc, from, target)
	}
	var command tea.Cmd
	mdl.rename, command = mdl.rename.Update(message)
	return mdl, command
}

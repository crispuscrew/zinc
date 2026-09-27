package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/keys"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	help       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	dim        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	selected   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	enumSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
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
	case modeRename:
		return mdl.renameView()
	case modeKeys:
		return mdl.keysView()
	default:
		return mdl.listView()
	}
}

func (mdl Model) listView() string {
	var output strings.Builder
	output.WriteString(titleStyle.Render("Zinc - apps") + "\n\n")
	if len(mdl.apps) == 0 {
		output.WriteString("no apps yet - press n to create one\n")
	}
	for index, row := range mdl.apps {
		cursor, state := "  ", "stopped"
		if index == mdl.cursor {
			cursor = "> "
		}
		if row.running {
			state = "running"
		}
		detail := oneLine(row.cfg.ImageMeta.Image, 96)
		if row.loadErr != nil {
			detail = errStyle.Render("invalid: " + oneLine(row.loadErr.Error(), 96))
		}
		line := fmt.Sprintf("%s%-16s %-7s %-10s %s", cursor, oneLine(row.name, 16), state, netLabel(row.cfg), detail)
		if index == mdl.cursor {
			line = selected.Render(line)
		}
		output.WriteString(line + "\n")
	}
	if mdl.err != nil {
		output.WriteString(errStyle.Render(oneLine(mdl.err.Error(), 512)) + "\n")
	} else if mdl.status != "" {
		output.WriteString(dim.Render(oneLine(mdl.status, 1024)) + "\n")
	}
	output.WriteString(mdl.listFooter())
	return output.String()
}

func (mdl Model) listFooter() string {
	var hints []string
	add := func(action keys.Action, label string) {
		if hint := mdl.keys.Scheme.HintPrimary(keys.CtxList, action); hint != "" {
			hints = append(hints, hint+" "+label)
		}
	}
	add(keys.New, "new")
	if row, ok := mdl.selected(); ok {
		if row.loadErr == nil {
			add(keys.Edit, "edit")
			if row.cfg.Type == schema.ZincContainer {
				add(keys.Rename, "rename")
			}
			if !row.running || row.cfg.StartConditions.Attached {
				add(keys.Run, "run")
			}
			if row.cfg.Type == schema.ZincContainer && row.cfg.StartConditions.Attached {
				add(keys.Shell, "shell")
			}
			if row.cfg.Type == schema.ZincVirtualization && row.running {
				add(keys.Shell, "console")
			}
			if row.running {
				add(keys.Stop, "stop")
				if row.cfg.Type == schema.ZincContainer {
					add(keys.Logs, "logs")
				}
			}
			if row.cfg.Type == schema.ZincContainer && len(row.cfg.ImageMeta.Install) > 0 {
				add(keys.Build, "build")
			}
		}
		add(keys.Delete, "delete")
	}
	add(keys.Refresh, "refresh")
	add(keys.Keys, "keys")
	add(keys.Quit, "quit")
	return help.Render(strings.Join(hints, " | "))
}

func netLabel(cfg schema.AppConfig) string {
	if len(cfg.NetworkMeta.Interfaces) == 0 {
		return "no NIC"
	}
	return fmt.Sprintf("net:%d", len(cfg.NetworkMeta.RulesByPriority))
}

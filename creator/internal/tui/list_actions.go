package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/keys"
)

func (mdl Model) handleListKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	action, ok := mdl.keys.Scheme.Resolve(keys.CtxList, message.String())
	if !ok {
		return mdl, nil
	}
	row, selected := mdl.selected()
	switch action {
	case keys.Quit:
		mdl.quitting = true
		return mdl, tea.Quit
	case keys.Up:
		mdl.cursor = max(mdl.cursor-1, 0)
	case keys.Down:
		mdl.cursor = min(mdl.cursor+1, max(len(mdl.apps)-1, 0))
	case keys.Refresh:
		return mdl, loadApps(mdl.svc)
	case keys.New:
		mdl.openForm(schema.AppConfig{}, true)
	case keys.Edit:
		if selected && row.loadErr == nil {
			mdl.openForm(row.raw, false)
			if row.cfg.Type == schema.ZincVirtualization {
				options, err := mdl.svc.LoadVM(row.cfg)
				if err != nil {
					mdl.form.err = err
				} else {
					mdl.form.loadVM(options)
				}
			}
		}
	case keys.Run:
		if selected {
			mdl.status = "launching " + row.name + "..."
			return mdl, launch(mdl.svc, row.name)
		}
	case keys.Shell:
		if selected {
			if row.cfg.Type == schema.ZincVirtualization {
				return mdl, console(mdl.svc, row.name)
			}
			if !row.cfg.StartConditions.Attached {
				mdl.status = row.name + ": a shell needs an Attached container app"
				return mdl, nil
			}
			return mdl, openShell(mdl.svc, row.name)
		}
	case keys.Build:
		if selected {
			if row.cfg.Type == schema.ZincVirtualization || (len(row.cfg.ImageMeta.Install) == 0 && len(row.cfg.CreatorFlags) == 0) {
				mdl.status = row.name + ": no derived image - nothing to build"
				return mdl, nil
			}
			return mdl, buildImage(mdl.svc, row.name)
		}
	case keys.Stop:
		if selected {
			return mdl, stop(mdl.svc, row.name)
		}
	case keys.Logs:
		if selected {
			return mdl, fetchLogs(mdl.svc, row.name)
		}
	case keys.Rename:
		if selected && row.loadErr == nil {
			mdl.rename, mdl.renameFrom = newInput(row.name, ""), row.name
			mdl.mode, mdl.status = modeRename, ""
			return mdl, mdl.rename.Focus()
		}
	case keys.Delete:
		if selected {
			mdl.confirmName, mdl.mode = row.name, modeConfirmDelete
		}
	case keys.Keys:
		mdl.mode, mdl.keysCursor, mdl.status = modeKeys, 0, ""
		return mdl, loadSchemes()
	}
	return mdl, nil
}

func (mdl *Model) openForm(cfg schema.AppConfig, creating bool) {
	mdl.form = newForm(cfg, creating)
	mdl.form.scheme, mdl.form.height = mdl.keys.Scheme, mdl.height
	mdl.mode, mdl.status = modeForm, ""
}

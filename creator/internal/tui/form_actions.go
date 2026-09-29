package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/creator/internal/advisory"
	"github.com/crispuscrew/zinc/creator/internal/keys"
)

func (mdl Model) handleKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if message.String() == "ctrl+c" {
		mdl.quitting = true
		return mdl, tea.Quit
	}
	switch mdl.mode {
	case modeForm:
		return mdl.handleFormKey(message)
	case modeLogs:
		if mdl.keys.Scheme.Is(keys.CtxLogs, keys.Back, message.String()) {
			mdl.mode = modeList
			return mdl, nil
		}
		var command tea.Cmd
		mdl.logs, command = mdl.logs.Update(message)
		return mdl, command
	case modeConfirmDelete:
		switch action, _ := mdl.keys.Scheme.Resolve(keys.CtxConfirm, message.String()); action {
		case keys.Yes:
			mdl.mode = modeList
			return mdl, remove(mdl.svc, mdl.confirmName)
		case keys.No:
			mdl.mode = modeList
		}
		return mdl, nil
	case modeRename:
		return mdl.handleRenameKey(message)
	case modeKeys:
		return mdl.handleKeysKey(message)
	default:
		return mdl.handleListKey(message)
	}
}

func (mdl Model) handleFormKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	command, result := mdl.form.update(message)
	switch result {
	case formCancel:
		mdl.mode, mdl.form = modeList, nil
	case formSave:
		return mdl.saveForm()
	case formEdit:
		cfg := mdl.form.toConfig()
		if mdl.form.err != nil {
			return mdl, nil
		}
		options, err := mdl.form.draftOptions(cfg)
		if err != nil {
			mdl.form.err = err
			return mdl, nil
		}
		if options != nil {
			mdl.form.vm = *options
		}
		return mdl, writeDraft(mdl.svc, cfg)
	case formResolve:
		mdl.status = "resolving image..."
		return mdl, resolveImage(mdl.svc, mdl.form.image.Value())
	}
	return mdl, command
}

func (mdl Model) saveForm() (tea.Model, tea.Cmd) {
	if !mdl.form.creating {
		if err := mdl.svc.CheckUnchanged(mdl.form.original, mdl.form.originalVM); err != nil {
			mdl.form.err = err
			return mdl, nil
		}
	}
	cfg := mdl.form.toConfig()
	if mdl.form.err != nil {
		return mdl, nil
	}
	options, err := mdl.form.options(cfg)
	if err != nil {
		mdl.form.err = err
		return mdl, nil
	}
	if err := mdl.svc.SaveDefinition(cfg, options, mdl.form.creating); err != nil {
		mdl.form.err = err
		return mdl, nil
	}
	mdl.mode, mdl.form = modeList, nil
	mdl.status, mdl.err = "saved "+cfg.AppNameID, nil
	if warnings := advisory.Summary(cfg); warnings != "" {
		mdl.status += "\nwarning: " + warnings
	}
	return mdl, loadApps(mdl.svc)
}

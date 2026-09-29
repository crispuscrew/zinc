// Package tui is the creator's keyboard-first app manager.
package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/keys"
)

type mode int

const (
	modeList mode = iota
	modeForm
	modeLogs
	modeConfirmDelete
	modeRename
	modeKeys
)

// Actions use the store key. Presentation uses resolved values; editing uses the
// authored document, so inherited fields never become explicit zero overrides.
type appRow struct {
	name     string
	cfg, raw schema.AppConfig
	running  bool
	loadErr  error
}

type Model struct {
	svc           backend.Service
	keys          keys.Active
	mode          mode
	apps          []appRow
	cursor        int
	form          *formModel
	logs          viewport.Model
	logsName      string
	logsReady     bool
	confirmName   string
	rename        textinput.Model
	renameFrom    string
	keysList      []string
	keysCursor    int
	width, height int
	status        string
	err           error
	quitting      bool
}

func New(service backend.Service, active keys.Active) Model {
	return Model{svc: service, keys: active, mode: modeList, logs: viewport.New(80, 20)}
}
func (mdl Model) Init() tea.Cmd { return loadApps(mdl.svc) }

func (mdl Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		mdl.width, mdl.height = message.Width, message.Height
		mdl.logs.Width, mdl.logs.Height = message.Width, max(message.Height-4, 3)
		mdl.logsReady = true
		if mdl.form != nil {
			mdl.form.height = message.Height
		}
	case appsMsg:
		mdl.apps = message.rows
		mdl.cursor = min(mdl.cursor, max(len(mdl.apps)-1, 0))
	case statusMsg:
		mdl.status, mdl.err = message.text, nil
		return mdl, loadApps(mdl.svc)
	case errMsg:
		mdl.err = message.err
	case logsMsg:
		mdl.logsName = message.name
		mdl.logs.SetContent(message.body)
		mdl.logs.GotoTop()
		mdl.mode = modeLogs
	case editReadyMsg:
		return mdl, openEditor(mdl.svc, message.path)
	case editedMsg:
		if mdl.form != nil {
			if message.err != nil {
				mdl.form.err = message.err
			} else {
				mdl.form.reload(message.cfg)
			}
		}
	case resolvedMsg:
		if mdl.form != nil {
			mdl.form.err = message.err
			if message.err == nil {
				mdl.form.image.SetValue(message.ref)
			}
		}
	case schemesMsg:
		mdl.keysList, mdl.keysCursor = message.names, indexOf(message.names, mdl.keys.Name)
	case schemeSetMsg:
		if message.err != nil {
			mdl.err = message.err
			return mdl, nil
		}
		mdl.keys, mdl.mode = message.active, modeList
		mdl.status, mdl.err = "keybinds: "+message.active.Name, nil
	case schemeEditMsg:
		return mdl, openSchemeEditor(message.path)
	case tea.KeyMsg:
		return mdl.handleKey(message)
	default:
		if mdl.mode == modeLogs {
			var command tea.Cmd
			mdl.logs, command = mdl.logs.Update(message)
			return mdl, command
		}
	}
	return mdl, nil
}

func (mdl Model) selected() (appRow, bool) {
	if mdl.cursor < 0 || mdl.cursor >= len(mdl.apps) {
		return appRow{}, false
	}
	return mdl.apps[mdl.cursor], true
}

func indexOf(names []string, wanted string) int {
	for index, name := range names {
		if name == wanted {
			return index
		}
	}
	return 0
}

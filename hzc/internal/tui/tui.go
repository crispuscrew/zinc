// Package tui is hzc's keyboard-first terminal UI (docs/architecture.md §9.1, M2).
//
// The Model + Update form the functional core: Update is a pure transition over
// (Model, Msg). All I/O — store reads/writes and podman exec — happens in tea.Cmd
// closures (commands.go) that drive the shared application service (core/app), so
// the decision logic is testable without a terminal and the TUI is a thin driving
// adapter over the hexagon.
//
// Scope (minimum but sufficient for M2): create / edit / delete / launch / stop /
// logs, end-to-end by keyboard. List-valued fields ([[mounts]], [keys], the pasta
// allowlist) stay TOML-editable and are shown read-only in the form; profiles are M10.
package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/core/app"
	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/hzc/internal/keys"
)

type mode int

const (
	modeList mode = iota
	modeForm
	modeLogs
	modeConfirmDelete
	modeKeys // keybind-scheme picker
)

type appRow struct {
	cfg     domain.AppConfig
	running bool
	loadErr error
}

// Model is the whole TUI state.
type Model struct {
	svc  app.Service // application facade (drives store/runtime/build/net)
	opts domain.HostOptions
	keys keys.Active // active keybind scheme (mdl.keys.Scheme drives key resolution)

	mode   mode
	apps   []appRow
	cursor int

	form *formModel

	logs      viewport.Model
	logsName  string
	logsReady bool

	confirmName string

	keysList   []string // scheme names shown in the picker (modeKeys)
	keysCursor int

	width, height int
	status        string
	err           error
	quitting      bool
}

// New builds the initial model. svc is the application service, opts carries the
// host env wiring for launches, and active is the resolved keybind scheme — all
// supplied by the caller (the composition root) so this package stays a thin
// adapter. A zero keys.Active falls back to the default scheme.
func New(svc app.Service, opts domain.HostOptions, active keys.Active) Model {
	return Model{svc: svc, opts: opts, keys: active, mode: modeList, logs: viewport.New(80, 20)}
}

func (mdl Model) Init() tea.Cmd { return loadApps(mdl.svc) }

func (mdl Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		mdl.width, mdl.height = msg.Width, msg.Height
		mdl.logs.Width = msg.Width
		mdl.logs.Height = max(msg.Height-4, 3)
		mdl.logsReady = true
		return mdl, nil

	case appsMsg:
		mdl.apps = msg.rows
		if mdl.cursor >= len(mdl.apps) {
			mdl.cursor = max(len(mdl.apps)-1, 0)
		}
		return mdl, nil

	case statusMsg:
		mdl.status, mdl.err = msg.text, nil
		return mdl, loadApps(mdl.svc) // refresh running indicators

	case errMsg:
		mdl.err = msg.err
		return mdl, nil

	case logsMsg:
		mdl.logsName = msg.name
		mdl.logs.SetContent(msg.body)
		mdl.logs.GotoTop()
		mdl.mode = modeLogs
		return mdl, nil

	case editReadyMsg:
		return mdl, openEditor(mdl.svc, msg.path)

	case editedMsg:
		if mdl.form == nil {
			return mdl, nil
		}
		if msg.err != nil {
			mdl.form.err = msg.err // bad edit: stay in the form, show why
			return mdl, nil
		}
		mdl.form.reload(msg.cfg)
		return mdl, nil

	case resolvedMsg:
		if mdl.form != nil {
			if msg.err != nil {
				mdl.form.err = msg.err
			} else {
				mdl.form.image.SetValue(msg.ref)
				mdl.form.err = nil
			}
		}
		return mdl, nil

	case schemesMsg:
		mdl.keysList = msg.names
		mdl.keysCursor = indexOf(msg.names, mdl.keys.Name) // land on the active scheme
		return mdl, nil

	case schemeSetMsg:
		if msg.err != nil {
			mdl.err = msg.err
			return mdl, nil
		}
		mdl.keys = msg.active
		mdl.mode = modeList
		mdl.status, mdl.err = "keybinds: "+msg.active.Name, nil
		return mdl, nil

	case schemeEditMsg:
		return mdl, openSchemeEditor(msg.path)

	case tea.KeyMsg:
		return mdl.handleKey(msg)
	}

	if mdl.mode == modeLogs {
		var cmd tea.Cmd
		mdl.logs, cmd = mdl.logs.Update(msg)
		return mdl, cmd
	}
	return mdl, nil
}

func (mdl Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		mdl.quitting = true
		return mdl, tea.Quit
	}

	switch mdl.mode {
	case modeForm:
		cmd, res := mdl.form.update(msg)
		switch res {
		case formCancel:
			mdl.mode, mdl.form = modeList, nil
			return mdl, nil
		case formSave:
			cfg := mdl.form.toConfig()
			if err := mdl.svc.Save(cfg); err != nil { // validates first
				mdl.form.err = err
				return mdl, nil
			}
			mdl.mode, mdl.form = modeList, nil
			mdl.status, mdl.err = "saved "+cfg.App.Name, nil
			return mdl, loadApps(mdl.svc)
		case formEdit:
			return mdl, writeDraft(mdl.svc, mdl.form.toConfig())
		case formResolve:
			mdl.status = "resolving image…"
			return mdl, resolveImage(mdl.svc, mdl.form.image.Value())
		}
		return mdl, cmd

	case modeLogs:
		if act, ok := mdl.keys.Scheme.Resolve(keys.CtxLogs, msg.String()); ok && act == keys.Back {
			mdl.mode = modeList
			return mdl, nil
		}
		var cmd tea.Cmd
		mdl.logs, cmd = mdl.logs.Update(msg)
		return mdl, cmd

	case modeConfirmDelete:
		switch act, _ := mdl.keys.Scheme.Resolve(keys.CtxConfirm, msg.String()); act {
		case keys.Yes:
			name := mdl.confirmName
			mdl.mode = modeList
			return mdl, remove(mdl.svc, name)
		case keys.No:
			mdl.mode = modeList
		}
		return mdl, nil

	case modeKeys:
		return mdl.handleKeysKey(msg)

	default:
		return mdl.handleListKey(msg)
	}
}

func (mdl Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	act, ok := mdl.keys.Scheme.Resolve(keys.CtxList, msg.String())
	if !ok {
		return mdl, nil
	}
	switch act {
	case keys.Quit:
		mdl.quitting = true
		return mdl, tea.Quit
	case keys.Up:
		if mdl.cursor > 0 {
			mdl.cursor--
		}
	case keys.Down:
		if mdl.cursor < len(mdl.apps)-1 {
			mdl.cursor++
		}
	case keys.Refresh:
		return mdl, loadApps(mdl.svc)
	case keys.New:
		mdl.form = newForm(domain.AppConfig{}, true)
		mdl.form.scheme = mdl.keys.Scheme
		mdl.mode, mdl.status = modeForm, ""
	case keys.Edit:
		if row, ok := mdl.selected(); ok && row.loadErr == nil {
			mdl.form = newForm(row.cfg, false)
			mdl.form.scheme = mdl.keys.Scheme
			mdl.mode, mdl.status = modeForm, ""
		}
	case keys.Run:
		if row, ok := mdl.selected(); ok {
			mdl.status = "launching " + row.cfg.App.Name + "…"
			return mdl, launch(mdl.svc, row.cfg.App.Name, mdl.opts)
		}
	case keys.Shell:
		if row, ok := mdl.selected(); ok {
			if !row.cfg.App.Multiterminal {
				mdl.status = row.cfg.App.Name + ": a shell needs a multiterminal app"
				return mdl, nil
			}
			mdl.status = "opening shell for " + row.cfg.App.Name + "…"
			return mdl, openShell(mdl.svc, row.cfg.App.Name, mdl.opts)
		}
	case keys.Build:
		if row, ok := mdl.selected(); ok {
			if row.cfg.App.Install == "" {
				mdl.status = row.cfg.App.Name + ": no install line — nothing to build"
				return mdl, nil
			}
			mdl.status = "building image for " + row.cfg.App.Name + "…"
			return mdl, buildImage(mdl.svc, row.cfg.App.Name)
		}
	case keys.Stop:
		if row, ok := mdl.selected(); ok {
			return mdl, stop(mdl.svc, row.cfg)
		}
	case keys.Logs:
		if row, ok := mdl.selected(); ok {
			return mdl, fetchLogs(mdl.svc, row.cfg.App.Name)
		}
	case keys.Delete:
		if row, ok := mdl.selected(); ok {
			mdl.confirmName = row.cfg.App.Name
			mdl.mode = modeConfirmDelete
		}
	case keys.Keys:
		mdl.mode, mdl.keysCursor, mdl.status = modeKeys, 0, ""
		return mdl, loadSchemes()
	}
	return mdl, nil
}

// handleKeysKey drives the scheme picker (modeKeys). Navigation reuses the active
// scheme's list movement (so vim users get j/k here too); enter applies the
// highlighted scheme, e edits/creates a custom copy, esc/q backs out.
func (mdl Model) handleKeysKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()
	switch keyStr {
	case "esc", "q":
		mdl.mode = modeList
		return mdl, nil
	case "enter":
		if len(mdl.keysList) == 0 {
			return mdl, nil
		}
		name := mdl.keysList[mdl.keysCursor]
		mdl.status = "switching to " + name + "…"
		return mdl, setScheme(name)
	case "e":
		if len(mdl.keysList) == 0 {
			return mdl, nil
		}
		return mdl, editScheme(mdl.keysList[mdl.keysCursor])
	}
	switch act, _ := mdl.keys.Scheme.Resolve(keys.CtxList, keyStr); act {
	case keys.Up:
		if mdl.keysCursor > 0 {
			mdl.keysCursor--
		}
	case keys.Down:
		if mdl.keysCursor < len(mdl.keysList)-1 {
			mdl.keysCursor++
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

// indexOf returns the position of want in names, or 0 if absent.
func indexOf(names []string, want string) int {
	for idx, name := range names {
		if name == want {
			return idx
		}
	}
	return 0
}

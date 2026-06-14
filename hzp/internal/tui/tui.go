// Package tui is hzp's keyboard-first terminal UI (docs/architecture.md §9.1, M2).
//
// The Model + Update form the functional core: Update is a pure transition over
// (Model, Msg). All I/O — store reads/writes and podman exec — happens in tea.Cmd
// closures (commands.go), so the decision logic is testable without a terminal.
//
// Scope (minimum but sufficient for M2): create / edit / delete / launch / stop /
// logs, end-to-end by keyboard. List-valued fields ([[mounts]], [keys], the pasta
// allowlist) stay TOML-editable and are shown read-only in the form; the egress
// allowlist itself is M3, and profiles are M10.
package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
	"github.com/crispuscrew/hyprzinc/hzp/internal/runspec"
	"github.com/crispuscrew/hyprzinc/hzp/internal/store"
)

type mode int

const (
	modeList mode = iota
	modeForm
	modeLogs
	modeConfirmDelete
)

type appRow struct {
	cfg     config.AppConfig
	running bool
	loadErr error
}

// Model is the whole TUI state.
type Model struct {
	st   *store.Store
	opts runspec.Options

	mode   mode
	apps   []appRow
	cursor int

	form *formModel

	logs      viewport.Model
	logsName  string
	logsReady bool

	confirmName string

	width, height int
	status        string
	err           error
	quitting      bool
}

// New builds the initial model. opts carries the host env wiring for launches,
// resolved by the caller (the imperative shell) so this package stays pure.
func New(st *store.Store, opts runspec.Options) Model {
	return Model{st: st, opts: opts, mode: modeList, logs: viewport.New(80, 20)}
}

func (m Model) Init() tea.Cmd { return loadApps(m.st) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.logs.Width = msg.Width
		m.logs.Height = max(msg.Height-4, 3)
		m.logsReady = true
		return m, nil

	case appsMsg:
		m.apps = msg.rows
		if m.cursor >= len(m.apps) {
			m.cursor = max(len(m.apps)-1, 0)
		}
		return m, nil

	case statusMsg:
		m.status, m.err = msg.text, nil
		return m, loadApps(m.st) // refresh running indicators

	case errMsg:
		m.err = msg.err
		return m, nil

	case logsMsg:
		m.logsName = msg.name
		m.logs.SetContent(msg.body)
		m.logs.GotoTop()
		m.mode = modeLogs
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.mode == modeLogs {
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	switch m.mode {
	case modeForm:
		cmd, res := m.form.update(msg)
		switch res {
		case formCancel:
			m.mode, m.form = modeList, nil
			return m, nil
		case formSave:
			cfg := m.form.toConfig()
			if err := m.st.Save(cfg); err != nil { // validates first
				m.form.err = err
				return m, nil
			}
			m.mode, m.form = modeList, nil
			m.status, m.err = "saved "+cfg.App.Name, nil
			return m, loadApps(m.st)
		}
		return m, cmd

	case modeLogs:
		switch msg.String() {
		case "esc", "q":
			m.mode = modeList
			return m, nil
		}
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		return m, cmd

	case modeConfirmDelete:
		switch msg.String() {
		case "y":
			name := m.confirmName
			m.mode = modeList
			return m, remove(m.st, name)
		case "n", "esc":
			m.mode = modeList
			return m, nil
		}
		return m, nil

	default:
		return m.handleListKey(msg)
	}
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.apps)-1 {
			m.cursor++
		}
	case "g", "ctrl+r":
		return m, loadApps(m.st)
	case "n":
		m.form = newForm(config.AppConfig{}, true)
		m.mode, m.status = modeForm, ""
	case "e", "enter":
		if row, ok := m.selected(); ok && row.loadErr == nil {
			m.form = newForm(row.cfg, false)
			m.mode, m.status = modeForm, ""
		}
	case "r":
		if row, ok := m.selected(); ok {
			m.status = "launching " + row.cfg.App.Name + "…"
			return m, launch(m.st, row.cfg.App.Name, m.opts)
		}
	case "s":
		if row, ok := m.selected(); ok {
			return m, stop(row.cfg.App.Name)
		}
	case "l":
		if row, ok := m.selected(); ok {
			return m, fetchLogs(row.cfg.App.Name)
		}
	case "d":
		if row, ok := m.selected(); ok {
			m.confirmName = row.cfg.App.Name
			m.mode = modeConfirmDelete
		}
	}
	return m, nil
}

func (m Model) selected() (appRow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.apps) {
		return appRow{}, false
	}
	return m.apps[m.cursor], true
}

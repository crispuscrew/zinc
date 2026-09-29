package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/keys"
	"strings"
)

type (
	appsMsg      struct{ rows []appRow }
	statusMsg    struct{ text string }
	errMsg       struct{ err error }
	logsMsg      struct{ name, body string }
	editReadyMsg struct{ path string }
	editedMsg    struct {
		cfg schema.AppConfig
		err error
	}
	resolvedMsg struct {
		ref string
		err error
	}
	schemesMsg   struct{ names []string }
	schemeSetMsg struct {
		active keys.Active
		err    error
	}
	schemeEditMsg struct{ path string }
)

func loadApps(service backend.Service) tea.Cmd {
	return func() tea.Msg {
		names, err := service.List()
		if err != nil {
			return errMsg{err}
		}
		running, _ := service.Running()
		rows := make([]appRow, 0, len(names))
		for _, name := range names {
			raw, err := service.Load(name)
			if err != nil {
				rows = append(rows, appRow{name: name, cfg: schema.AppConfig{AppNameID: name}, loadErr: err})
				continue
			}
			cfg, err := service.LoadResolved(name)
			if err != nil {
				cfg = raw
			}
			rows = append(rows, appRow{name: name, cfg: cfg, raw: raw, running: running[name], loadErr: err})
		}
		return appsMsg{rows}
	}
}

func action(service backend.Service, verb, name, label string, shell bool) tea.Cmd {
	return func() tea.Msg {
		report, err := service.Action(verb, name, shell)
		if err != nil {
			return errMsg{err}
		}
		return statusMsg{strings.TrimSpace(label + " " + name + "\n" + report)}
	}
}

func launch(service backend.Service, name string) tea.Cmd {
	return action(service, "run", name, "launched", false)
}
func openShell(service backend.Service, name string) tea.Cmd {
	return action(service, "term", name, "opened shell for", true)
}
func console(service backend.Service, name string) tea.Cmd {
	return action(service, "term", name, "console for", false)
}
func buildImage(service backend.Service, name string) tea.Cmd {
	return action(service, "build", name, "built image for", false)
}
func stop(service backend.Service, name string) tea.Cmd {
	return action(service, "stop", name, "stopped", false)
}

func resolveImage(service backend.Service, reference string) tea.Cmd {
	return func() tea.Msg { pinned, err := service.Resolve(reference); return resolvedMsg{pinned, err} }
}

func renameApp(service backend.Service, from, target string) tea.Cmd {
	return func() tea.Msg {
		if err := service.Rename(from, target); err != nil {
			return errMsg{err}
		}
		return statusMsg{"renamed " + from + " -> " + target}
	}
}

func remove(service backend.Service, name string) tea.Cmd {
	return func() tea.Msg {
		if err := service.Delete(name); err != nil {
			return errMsg{err}
		}
		return statusMsg{"deleted " + name}
	}
}

func fetchLogs(service backend.Service, name string) tea.Cmd {
	return func() tea.Msg {
		body, err := service.Logs(name)
		if err != nil {
			body = strings.TrimRight(body, "\n") + "\n(" + err.Error() + ")"
		}
		if strings.TrimSpace(body) == "" {
			body = "(no output)"
		}
		return logsMsg{name, body}
	}
}

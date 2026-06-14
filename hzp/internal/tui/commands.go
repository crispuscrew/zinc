package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
	"github.com/crispuscrew/hyprzinc/hzp/internal/runspec"
	"github.com/crispuscrew/hyprzinc/hzp/internal/store"
)

// Messages produced by the I/O commands below. Update consumes them.
type (
	appsMsg   struct{ rows []appRow }
	statusMsg struct{ text string }
	errMsg    struct{ err error }
	logsMsg   struct {
		name string
		body string
	}
)

// loadApps lists the store and tags each app with its running state (one
// `podman ps`). A bad definition is surfaced per-row rather than failing the load.
func loadApps(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		names, err := st.List()
		if err != nil {
			return errMsg{err}
		}
		running := runningSet()
		rows := make([]appRow, 0, len(names))
		for _, n := range names {
			cfg, err := st.Load(n)
			if err != nil {
				rows = append(rows, appRow{cfg: config.AppConfig{App: config.App{Name: n}}, loadErr: err})
				continue
			}
			rows = append(rows, appRow{cfg: cfg, running: running[n]})
		}
		return appsMsg{rows}
	}
}

// runningSet is the set of container names podman currently reports as running.
func runningSet() map[string]bool {
	set := map[string]bool{}
	out, err := exec.Command("podman", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return set
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set
}

// launch validates the app and starts its container detached from the TUI's
// stdio, so a GUI app opens its own window without disturbing the terminal.
func launch(st *store.Store, name string, opts runspec.Options) tea.Cmd {
	return func() tea.Msg {
		cfg, err := st.Load(name)
		if err != nil {
			return errMsg{err}
		}
		if err := config.Validate(cfg); err != nil {
			return errMsg{fmt.Errorf("%s: %w", name, err)}
		}
		args, err := runspec.BuildArgs(cfg, opts)
		if err != nil {
			return errMsg{err}
		}
		c := exec.Command("podman", args...) // stdio left nil → /dev/null
		if err := c.Start(); err != nil {
			return errMsg{fmt.Errorf("launch %s: %w", name, err)}
		}
		go c.Wait() // reap when it exits; don't block the UI
		return statusMsg{"launched " + name}
	}
}

func stop(name string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("podman", runspec.StopArgs(name)...).CombinedOutput()
		if err != nil {
			return errMsg{fmt.Errorf("stop %s: %s", name, strings.TrimSpace(string(out)))}
		}
		return statusMsg{"stopped " + name}
	}
}

func remove(st *store.Store, name string) tea.Cmd {
	return func() tea.Msg {
		if err := st.Delete(name); err != nil {
			return errMsg{err}
		}
		return statusMsg{"deleted " + name}
	}
}

// fetchLogs grabs the last lines of the container's logs. podman may exit nonzero
// (e.g. container never ran) but still print useful output, so we keep both.
func fetchLogs(name string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("podman", "logs", "--tail", "500", name).CombinedOutput()
		body := string(out)
		if err != nil {
			body = strings.TrimRight(body, "\n") + "\n(" + err.Error() + ")"
		}
		if strings.TrimSpace(body) == "" {
			body = "(no output)"
		}
		return logsMsg{name: name, body: body}
	}
}

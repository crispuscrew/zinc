package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/core/app"
	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/hzc/internal/keys"
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
	editReadyMsg struct{ path string } // temp TOML written; ready to open $EDITOR
	editedMsg    struct {              // $EDITOR returned; carries the re-parsed config
		cfg domain.AppConfig
		err error
	}
	resolvedMsg struct { // image field resolved to a pinned @sha256 reference
		ref string
		err error
	}
	schemesMsg   struct{ names []string } // available keybind schemes (picker)
	schemeSetMsg struct {                 // a scheme was activated and reloaded
		active keys.Active
		err    error
	}
	schemeEditMsg struct{ path string } // a scheme file is ready to open in $EDITOR
)

// resolveImage pins the image field's tag to its @sha256 digest (§5.5) via the
// service's image resolver — which pulls the image, so it runs off the UI thread.
func resolveImage(svc app.Service, ref string) tea.Cmd {
	return func() tea.Msg {
		pinned, err := svc.Resolve(ref)
		return resolvedMsg{ref: pinned, err: err}
	}
}

// writeDraft serializes the form's current config to a temp .toml so $EDITOR can
// open it; the editor launch is the follow-up (openEditor) once the file exists.
func writeDraft(svc app.Service, cfg domain.AppConfig) tea.Cmd {
	return func() tea.Msg {
		data, err := svc.Marshal(cfg)
		if err != nil {
			return errMsg{err}
		}
		file, err := os.CreateTemp("", "hzc-*.toml")
		if err != nil {
			return errMsg{err}
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			os.Remove(file.Name())
			return errMsg{err}
		}
		file.Close()
		return editReadyMsg{path: file.Name()}
	}
}

// openEditor hands the temp file to $EDITOR (default vim), releasing the terminal to
// it and restoring the TUI on exit, then re-parses the (possibly edited) TOML.
// Parsing here keeps Update free of I/O — it just consumes the result.
func openEditor(svc app.Service, path string) tea.Cmd {
	cmd := exec.Command(editorArgv(path)[0], editorArgv(path)[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return editedMsg{err: fmt.Errorf("editor: %w", err)}
		}
		cfg, lerr := svc.LoadFile(path)
		return editedMsg{cfg: cfg, err: lerr}
	})
}

// editorArgv builds the $EDITOR command (default vim) ending in path.
func editorArgv(path string) []string {
	argv := strings.Fields(os.Getenv("EDITOR"))
	if len(argv) == 0 {
		argv = []string{"vim"}
	}
	return append(argv, path)
}

// loadSchemes lists the selectable keybind schemes (built-ins + custom files).
func loadSchemes() tea.Cmd {
	return func() tea.Msg {
		kst, err := keys.DefaultStore()
		if err != nil {
			return errMsg{err}
		}
		names, err := kst.List()
		if err != nil {
			return errMsg{err}
		}
		return schemesMsg{names}
	}
}

// setScheme persists the chosen scheme as active (validating it first) and returns
// the reloaded effective bindings for the running session.
func setScheme(name string) tea.Cmd {
	return func() tea.Msg {
		kst, err := keys.DefaultStore()
		if err != nil {
			return schemeSetMsg{err: err}
		}
		if err := kst.SetActive(name); err != nil {
			return schemeSetMsg{err: err}
		}
		active, err := kst.Load()
		return schemeSetMsg{active: active, err: err}
	}
}

// editScheme ensures an editable custom scheme file exists (scaffolding a copy when
// the selection is a built-in) and hands its path to the editor step.
func editScheme(name string) tea.Cmd {
	return func() tea.Msg {
		kst, err := keys.DefaultStore()
		if err != nil {
			return errMsg{err}
		}
		_, path, err := kst.EnsureEditable(name)
		if err != nil {
			return errMsg{err}
		}
		return schemeEditMsg{path: path}
	}
}

// openSchemeEditor opens a scheme file in $EDITOR, then refreshes the picker list so
// a newly created custom scheme shows up. Unlike app TOMLs the file is kept (it's
// the user's config) and validated only when activated.
func openSchemeEditor(path string) tea.Cmd {
	cmd := exec.Command(editorArgv(path)[0], editorArgv(path)[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return errMsg{fmt.Errorf("editor: %w", err)}
		}
		kst, derr := keys.DefaultStore()
		if derr != nil {
			return errMsg{derr}
		}
		names, lerr := kst.List()
		if lerr != nil {
			return errMsg{lerr}
		}
		return schemesMsg{names}
	})
}

// loadApps lists the store and tags each app with its running state (one runtime
// query). A bad definition is surfaced per-row rather than failing the load.
func loadApps(svc app.Service) tea.Cmd {
	return func() tea.Msg {
		names, err := svc.List()
		if err != nil {
			return errMsg{err}
		}
		running, _ := svc.Running() // a query failure degrades to "nothing running"
		rows := make([]appRow, 0, len(names))
		for _, name := range names {
			cfg, err := svc.Load(name)
			if err != nil {
				rows = append(rows, appRow{cfg: domain.AppConfig{App: domain.App{Name: name}}, loadErr: err})
				continue
			}
			rows = append(rows, appRow{cfg: cfg, running: running[name]})
		}
		return appsMsg{rows}
	}
}

// launch hands the app to the shared application service, which validates, applies
// the egress lock-down before the app starts, and detaches it — the same path hzl
// uses, so the TUI carries no launch logic of its own (§9.1).
func launch(svc app.Service, name string, opts domain.HostOptions) tea.Cmd {
	return func() tea.Msg {
		cfg, err := svc.Load(name)
		if err != nil {
			return errMsg{err}
		}
		if err := svc.Launch(cfg, opts); err != nil {
			return errMsg{err}
		}
		return statusMsg{"launched " + name}
	}
}

// openShell opens another terminal as a shell for a multiterminal app. Like launch
// it returns immediately — the service spawns a detached waiter that owns the
// terminal's lifecycle (§9.1).
func openShell(svc app.Service, name string, opts domain.HostOptions) tea.Cmd {
	return func() tea.Msg {
		cfg, err := svc.Load(name)
		if err != nil {
			return errMsg{err}
		}
		if err := svc.OpenTerminal(cfg, opts, true); err != nil {
			return errMsg{err}
		}
		return statusMsg{"opened shell for " + name}
	}
}

// buildImage force-rebuilds an app's derived image (FROM app.image + the install
// layer). This is the explicit-rebuild action; a plain run already rebuilds on
// demand when the install line or base changes (§9.1).
func buildImage(svc app.Service, name string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := svc.Load(name)
		if err != nil {
			return errMsg{err}
		}
		if err := svc.Build(cfg); err != nil {
			return errMsg{err}
		}
		return statusMsg{"built image for " + name}
	}
}

// stop tears down a running app via the service (the pod and its filtered netns for
// a pasta app, the container otherwise).
func stop(svc app.Service, cfg domain.AppConfig) tea.Cmd {
	return func() tea.Msg {
		if err := svc.Stop(cfg); err != nil {
			return errMsg{err}
		}
		return statusMsg{"stopped " + cfg.App.Name}
	}
}

func remove(svc app.Service, name string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.Delete(name); err != nil {
			return errMsg{err}
		}
		return statusMsg{"deleted " + name}
	}
}

// fetchLogs grabs the last lines of the container's logs. podman may exit nonzero
// (e.g. container never ran) but still print useful output, so we keep both.
func fetchLogs(svc app.Service, name string) tea.Cmd {
	return func() tea.Msg {
		body, err := svc.Logs(name, 500)
		if err != nil {
			body = strings.TrimRight(body, "\n") + "\n(" + err.Error() + ")"
		}
		if strings.TrimSpace(body) == "" {
			body = "(no output)"
		}
		return logsMsg{name: name, body: body}
	}
}

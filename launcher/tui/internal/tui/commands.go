package tui

import tea "github.com/charmbracelet/bubbletea"

// Runner is the slice of the zcr delegate the picker drives. Keeping it an interface
// lets a fake stand in for tests (the real one shells out to the zcr binary).
type Runner interface {
	Launch(name string) error
	Running() (map[string]bool, error)
}

// runningMsg carries the set of apps zcr reports as up, for the running indicator. It is
// best-effort: on error the picker simply shows no indicators.
type runningMsg struct {
	running map[string]bool
	err     error
}

// launchedMsg is the result of launching an app through zcr.
type launchedMsg struct {
	name string
	err  error
}

// loadRunning asks zcr which apps are up (see runningMsg). Runs off the UI goroutine.
func loadRunning(runner Runner) tea.Cmd {
	return func() tea.Msg {
		running, err := runner.Running()
		return runningMsg{running: running, err: err}
	}
}

// launch runs `zcr run <name> --exec` (see launchedMsg). Runs off the UI goroutine.
func launch(runner Runner, name string) tea.Cmd {
	return func() tea.Msg {
		return launchedMsg{name: name, err: runner.Launch(name)}
	}
}

// Command zlt is the Zinc launcher (TUI): a fast, keyboard-first fuzzy picker over the
// defined apps (~/.config/zinc/apps). It lists what zcc authored and shells out to the
// `zcr` binary to run the chosen app - it never imports the runtime (the same split zcc
// uses). Run it two ways:
//
//	zlt            open the picker (type to filter, enter launches, esc quits)
//	zlt <app>      launch a defined app directly (for a desktop hotkey or a script)
//
// Dependency auto-start, the network lock-down, and derived-image builds are all zcr's
// job, so the launcher stays a thin front-end (docs/architecture.md 9.3).
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/zinc/launcher/tui/internal/runner"
	"github.com/crispuscrew/zinc/launcher/tui/internal/store"
	"github.com/crispuscrew/zinc/launcher/tui/internal/tui"
)

// version is the release, stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zlt: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	switch {
	case len(argv) == 1 && (argv[0] == "-h" || argv[0] == "--help"):
		fmt.Println(usage)
		return nil
	case len(argv) == 1 && (argv[0] == "version" || argv[0] == "--version"):
		fmt.Println("zlt " + versionString())
		return nil
	case len(argv) == 1:
		return launchDirect(argv[0]) // zlt <app>
	case len(argv) == 0:
		return pick() // zlt
	default:
		return fmt.Errorf("too many arguments\n%s", usage)
	}
}

const usage = "usage:\n" +
	"  zlt            open the app picker (type to filter, enter launches, esc quits)\n" +
	"  zlt <app>      launch a defined app directly\n" +
	"  zlt --version"

// zcrDelegate adapts the runner package to the tui.Runner interface.
type zcrDelegate struct{}

func (zcrDelegate) Launch(name string) error          { return runner.Launch(name) }
func (zcrDelegate) Running() (map[string]bool, error) { return runner.Running() }

// launchDirect runs a named app straight through zcr, with no UI - for a hotkey binding.
func launchDirect(name string) error {
	if err := runner.Launch(name); err != nil {
		return err
	}
	fmt.Println("launched " + name)
	return nil
}

// pick loads the defined apps and opens the picker; on selection it has already launched
// through zcr, so we just report what came up.
func pick() error {
	apps, err := loadApps()
	if err != nil {
		return err
	}
	model, err := tea.NewProgram(tui.New(apps, zcrDelegate{})).Run()
	if err != nil {
		return err
	}
	if mdl, ok := model.(tui.Model); ok && mdl.Launched() != "" {
		fmt.Println("launched " + mdl.Launched())
	}
	return nil
}

// loadApps reads every defined app for display. A file that fails to decode is still
// listed by name (launching it will surface zcr's validation error) rather than hidden.
func loadApps() ([]tui.App, error) {
	sto, err := store.Default()
	if err != nil {
		return nil, err
	}
	names, err := sto.List()
	if err != nil {
		return nil, err
	}
	apps := make([]tui.App, 0, len(names))
	for _, name := range names {
		app := tui.App{Name: name}
		if cfg, err := sto.Load(name); err == nil {
			app.Description = cfg.Description
		}
		apps = append(apps, app)
	}
	sort.Slice(apps, func(left, right int) bool { return apps[left].Name < apps[right].Name })
	return apps, nil
}

// versionString returns the ldflags-stamped version, falling back to the module version
// recorded in the build info (set when installed via `go install ...@vX.Y.Z`).
func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

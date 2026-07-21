// Command zlg is the Zinc launcher (GUI): a graphical picker over the defined apps
// (~/.config/zinc/apps). It is the point-and-click sibling of zlt - it lists what zcc
// authored, filters as you type, and shells out to the `zcr` binary to run the chosen app.
// Like zcc and zlt it never imports the runtime; it and zcr meet only at the on-disk YAML
// format and the process boundary. Run it two ways:
//
//	zlg            open the picker window (type to filter, enter launches, esc quits)
//	zlg <app>      launch a defined app directly (for a desktop hotkey or a script)
//
// zlg renders in pure Go (the Wayland wire protocol plus a software-drawn buffer), so it
// stays a static, dependency-light binary like the other tools. Dependency auto-start, the
// network lock-down, and derived-image builds are all zcr's job.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"sort"

	"github.com/crispuscrew/zinc/launcher/common/runner"
	"github.com/crispuscrew/zinc/launcher/common/store"
	"github.com/crispuscrew/zinc/launcher/gui/internal/picker"
	"github.com/crispuscrew/zinc/launcher/gui/internal/ui"
)

// version is the release, stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zlg: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	switch {
	case len(argv) == 1 && (argv[0] == "-h" || argv[0] == "--help"):
		fmt.Println(usage)
		return nil
	case len(argv) == 1 && (argv[0] == "version" || argv[0] == "--version"):
		fmt.Println("zlg " + versionString())
		return nil
	case len(argv) == 1:
		return launchDirect(argv[0]) // zlg <app>
	case len(argv) == 0:
		return pick() // zlg
	default:
		return fmt.Errorf("too many arguments\n%s", usage)
	}
}

const usage = "usage:\n" +
	"  zlg            open the app picker window (type to filter, enter launches, esc quits)\n" +
	"  zlg <app>      launch a defined app directly\n" +
	"  zlg --version"

// zcrDelegate adapts the runner package to the ui.Runner interface.
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

// pick loads the defined apps and opens the picker window; on selection it has already
// launched through zcr, so we just report what came up.
func pick() error {
	apps, err := loadApps()
	if err != nil {
		return err
	}
	launched, err := ui.Run(apps, zcrDelegate{})
	if err != nil {
		return err
	}
	if launched != "" {
		fmt.Println("launched " + launched)
	}
	return nil
}

// loadApps reads every defined app for display. A file that fails to decode is still
// listed by name (launching it will surface zcr's validation error) rather than hidden.
func loadApps() ([]picker.App, error) {
	sto, err := store.Default()
	if err != nil {
		return nil, err
	}
	names, err := sto.List()
	if err != nil {
		return nil, err
	}
	apps := make([]picker.App, 0, len(names))
	for _, name := range names {
		app := picker.App{Name: name}
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

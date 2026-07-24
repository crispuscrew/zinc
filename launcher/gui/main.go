// Command zlg is the Zinc launcher (GUI): a graphical picker over the defined apps
// (~/.config/zinc/apps). It is the point-and-click sibling of zlt - it lists what zcc
// authored, filters as you type, and shells out to the `zcr` binary to run the chosen app.
// Like zcc and zlt it never imports the runtime; it and zcr meet only at the on-disk YAML
// format and the process boundary. Run it two ways:
//
//	zlg            open the picker window (type to filter, enter launches, esc quits)
//	zlg <app>      launch a defined app directly (for a desktop hotkey or a script)
//
// The picker window itself is the reusable `menu` module (a pure-Go Wayland layer-shell
// overlay); zlg is a thin consumer that supplies the app list and an activate callback. So
// zlg stays a static, dependency-light binary, and other programs can build their own menus
// over the same core. Dependency auto-start, the network lock-down, and derived-image builds
// are all zcr's job.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strconv"

	"github.com/crispuscrew/zinc/launcher/common/runner"
	"github.com/crispuscrew/zinc/launcher/common/store"
	"github.com/crispuscrew/zinc/menu"
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

// launchDirect runs a named app straight through zcr, with no UI - for a hotkey binding.
func launchDirect(name string) error {
	if err := runner.Launch(name); err != nil {
		return err
	}
	fmt.Println("launched " + name)
	return nil
}

// pick loads the defined apps and opens the menu overlay. The activate callback launches the
// chosen app through zcr from inside the overlay, so a launch error is shown in the window
// (the overlay stays open) rather than tearing it down.
func pick() error {
	items, err := loadItems()
	if err != nil {
		return err
	}
	activate := func(item menu.Item) error {
		if err := runner.Launch(item.Label); err != nil {
			return fmt.Errorf("cannot launch %s - %w", item.Label, err)
		}
		return nil
	}
	index, err := menu.Run(items, activate, menuOptions())
	if err != nil {
		return err
	}
	if index >= 0 {
		fmt.Println("launched " + items[index].Label)
	}
	return nil
}

// loadItems reads every defined app as a menu item, marking the ones zcr reports running. A
// file that fails to decode is still listed by name (launching it will surface zcr's
// validation error) rather than hidden.
func loadItems() ([]menu.Item, error) {
	sto, err := store.Default()
	if err != nil {
		return nil, err
	}
	names, err := sto.List()
	if err != nil {
		return nil, err
	}
	running, _ := runner.Running() // best-effort; the picker still works without zcr
	items := make([]menu.Item, 0, len(names))
	for _, name := range names {
		item := menu.Item{Label: name, Marked: running[name]}
		if cfg, err := sto.Load(name); err == nil {
			item.Description = cfg.Description
			item.Group = cfg.Group
			item.Icon = cfg.Icon
		}
		items = append(items, item)
	}
	// Order by group then name (ungrouped last), so the menu draws one header per group and
	// the ungrouped apps fall under a trailing "Other" section.
	sort.Slice(items, func(left, right int) bool {
		leftGroup, rightGroup := items[left].Group, items[right].Group
		if leftGroup != rightGroup {
			if leftGroup == "" {
				return false
			}
			if rightGroup == "" {
				return true
			}
			return leftGroup < rightGroup
		}
		return items[left].Label < items[right].Label
	})
	return items, nil
}

// menuOptions maps zlg's env knobs onto the menu Options: ZLG_OPACITY (a 0..100 percentage),
// ZLG_NO_ANIM (disable the fade-in), and ZLG_DEBUG (trace the Wayland handshake). The app-id
// lets tiling compositors match window rules against zlg.
func menuOptions() menu.Options {
	opts := menu.Options{
		Prompt:   "> ",
		Footer:   "up/down move   enter launch   esc quit",
		AppID:    "zinc.launcher",
		FontPath: os.Getenv("ZLG_FONT"), // pin a specific font; empty auto-detects a system Nerd Font
		NoAnim:   os.Getenv("ZLG_NO_ANIM") != "",
		Debug:    os.Getenv("ZLG_DEBUG") != "",
	}
	if raw := os.Getenv("ZLG_OPACITY"); raw != "" {
		if percent, err := strconv.Atoi(raw); err == nil && percent >= 0 && percent <= 100 {
			opts.Opacity = float64(percent) / 100
		}
	}
	return opts
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

// Command zlg is the Zinc launcher (GUI): a graphical picker over the defined apps
// (~/.config/zinc/apps). It is the point-and-click sibling of zlt - it lists what zc
// authored, filters as you type, and shells out to `zcr` or `zvr` to run the chosen app.
// Like zc and zlt it never imports the runtime; they meet only at the on-disk YAML
// format and the process boundary. Run it two ways:
//
//	zlg            open the picker window (type to filter, enter launches, esc quits)
//	zlg <app>      launch a defined app directly (for a desktop hotkey or a script)
//
// The picker window itself is the reusable `menu` module (a pure-Go Wayland layer-shell
// overlay); zlg is a thin consumer that supplies the app list and an activate callback. So
// zlg stays a static, dependency-light binary, and other programs can build their own menus
// over the same core. Dependency auto-start, the network lock-down, and derived-image builds
// are the selected runtime's job.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/launcher/common/runner"
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

// launchDirect runs a named app through its runtime, with no UI - for a hotkey binding.
func launchDirect(name string) error {
	if err := runner.Launch(name); err != nil {
		return err
	}
	fmt.Println("launched " + name)
	return nil
}

// pick loads the defined apps and opens the menu overlay. The activate callback launches the
// chosen app through its runtime, so a launch error is shown in the window
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

// menuOptions maps zlg's env knobs onto the menu Options: ZLG_OPACITY (background
// translucency), ZLG_FONT (pin a font), ZLG_NO_ANIM (disable the fade-in), and ZLG_DEBUG
// (trace the Wayland handshake). The app-id lets tiling compositors match window rules
// against zlg.
func menuOptions() menu.Options {
	opts := menu.Options{
		Prompt:   "> ",
		Footer:   "up/down move   enter launch   esc quit",
		BusyVerb: "launching", // the banner while zcr starts the app: "launching nvim..."
		AppID:    "zinc.launcher",
		FontPath: os.Getenv("ZLG_FONT"), // pin a specific font; empty auto-detects a system Nerd Font
		NoAnim:   os.Getenv("ZLG_NO_ANIM") != "",
		Debug:    os.Getenv("ZLG_DEBUG") != "",
	}
	if raw := os.Getenv("ZLG_OPACITY"); raw != "" {
		opacity, ok := parseOpacity(raw)
		if !ok {
			// Warn rather than ignore silently: a typo here is otherwise indistinguishable
			// from the overlay simply not supporting translucency.
			fmt.Fprintf(os.Stderr, "zlg: ignoring ZLG_OPACITY=%q - want a percentage (0-100, e.g. 20) or a fraction (0-1, e.g. 0.2)\n", raw)
		}
		opts.Opacity = opacity
	}
	return opts
}

// parseOpacity reads a ZLG_OPACITY value in either form people reach for: a percentage
// ("20") or a fraction ("0.2"). A value above 1 is read as a percentage, at or below 1 as a
// fraction - so "1" and "100" both mean fully opaque and there is no reading under which a
// plausible input silently becomes an invisible window. It reports false for a value it
// cannot use, in which case the returned 0 leaves the overlay opaque.
func parseOpacity(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value < 0 || value > 100 {
		return 0, false
	}
	if value > 1 {
		value /= 100
	}
	return value, true
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

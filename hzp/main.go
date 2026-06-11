// Command hzp is HyprZinc's container-definition tool (docs/architecture.md §9.1).
//
// M0 surface (imperative shell around the config/runspec functional core):
//
//	hzp validate <app.toml>          # parse + validate, report problems
//	hzp run <app.toml> [--exec]      # build the podman command; print it, or run it
//
// The TUI, the config store, and full lifecycle management arrive in later
// milestones (see ROADMAP.md).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
	"github.com/crispuscrew/hyprzinc/hzp/internal/runspec"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hzp: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) < 2 {
		return fmt.Errorf("usage: hzp <validate|run> <app.toml> [--exec]")
	}
	command, path := argv[0], argv[1]

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if validationErr := config.Validate(cfg); validationErr != nil {
		return fmt.Errorf("invalid config %s:\n%w", path, validationErr)
	}

	switch command {
	case "validate":
		preset := cfg.App.Preset
		if preset == "" {
			preset = "(none)"
		}
		fmt.Printf("ok: %s — image=%s preset=%s network=%s\n",
			cfg.App.Name, cfg.App.Image, preset, cfg.Network.Mode)
		return nil

	case "run":
		args, err := runspec.BuildArgs(cfg, optionsFromEnv())
		if err != nil {
			return err
		}
		if !contains(argv[2:], "--exec") {
			fmt.Println("podman " + strings.Join(shellQuote(args), " "))
			return nil
		}
		process := exec.Command("podman", args...)
		process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
		return process.Run()

	default:
		return fmt.Errorf("unknown command %q (want validate|run)", command)
	}
}

func optionsFromEnv() runspec.Options {
	return runspec.Options{
		RuntimeDir:     os.Getenv("XDG_RUNTIME_DIR"),
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		ThemeBundleDir: os.Getenv("HYPRZINC_THEME_BUNDLE"),
		HomeDir:        "/root",
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// safeBareToken matches args that need no quoting to survive a POSIX shell.
var safeBareToken = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote renders argv as a copy-pasteable POSIX shell command line. It is for
// DISPLAY ONLY: `hzp run` without --exec prints this so the command can be eyeballed
// or pasted. Real execution uses exec.Command(argv...) directly — no shell, no
// quoting — so this is about the honesty of the printed line, not execution safety.
// Already-safe tokens are left bare; anything else is single-quoted with embedded
// single quotes escaped as '\” , which is correct to paste.
func shellQuote(args []string) []string {
	out := make([]string, len(args))
	for index, arg := range args {
		if arg != "" && safeBareToken.MatchString(arg) {
			out[index] = arg
			continue
		}
		out[index] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return out
}

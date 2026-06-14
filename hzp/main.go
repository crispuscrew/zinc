// Command hzp is HyprZinc's container-definition tool (docs/architecture.md §9.1).
//
// It is the imperative shell around the config/store/runspec functional core:
//
//	hzp new <name> --image <img> [--preset strict|standard|networked]
//	hzp list                          # defined apps
//	hzp validate <name|app.toml>      # parse + validate, report problems
//	hzp delete <name>
//	hzp run <name|app.toml> [--exec]  # build the podman command; print it, or launch
//	hzp stop|restart|inspect <name>
//	hzp logs <name> [-f]
//
// A bare <name> resolves against the store (~/.config/hyprzinc/apps); an argument
// that looks like a path (contains "/" or ends in ".toml") is read directly. The
// keyboard-first TUI arrives in M2.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/hzp/internal/config"
	"github.com/crispuscrew/hyprzinc/hzp/internal/runspec"
	"github.com/crispuscrew/hyprzinc/hzp/internal/store"
	"github.com/crispuscrew/hyprzinc/hzp/internal/tui"
)

const usage = `usage: hzp <command> [args]

  tui                               keyboard-first manager (create/edit/run/stop/logs)
  new <name> --image <img> [--preset strict|standard|networked] [--desc d] [--icon i]
  list
  validate <name|app.toml>
  delete <name>
  run <name|app.toml> [--exec]
  stop|restart|inspect <name>
  logs <name> [-f]`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hzp: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) < 1 {
		return fmt.Errorf("%s", usage)
	}
	cmd, rest := argv[0], argv[1:]
	switch cmd {
	case "tui":
		return cmdTUI()
	case "new":
		return cmdNew(rest)
	case "list":
		return cmdList()
	case "validate":
		return cmdValidate(rest)
	case "delete":
		return cmdDelete(rest)
	case "run":
		return cmdRun(rest)
	case "stop", "restart", "inspect":
		return cmdLifecycle(cmd, rest)
	case "logs":
		return cmdLogs(rest)
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

func cmdTUI() error {
	st, err := store.Default()
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(tui.New(st, optionsFromEnv()), tea.WithAltScreen()).Run()
	return err
}

func cmdNew(argv []string) error {
	// The name is the first argument; flags follow it (Go's flag parser stops at
	// the first positional, so "new <name> --image …" must split this way).
	if len(argv) < 1 || strings.HasPrefix(argv[0], "-") {
		return fmt.Errorf("usage: hzp new <name> --image <img> [--preset strict|standard|networked]")
	}
	name, flags := argv[0], argv[1:]

	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	image := fs.String("image", "", "container image (digest-pinned for third-party; §5.5)")
	preset := fs.String("preset", config.PresetDefaultNew, "preset template: strict|standard|networked")
	desc := fs.String("desc", "", "human-readable description")
	icon := fs.String("icon", "", "icon name")
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q (flags must follow the name)", fs.Arg(0))
	}
	cfg, ok := config.DefaultsFor(*preset)
	if !ok {
		return fmt.Errorf("unknown preset %q (want strict|standard|networked)", *preset)
	}
	cfg.App.Name = name
	cfg.App.Image = *image
	cfg.App.Description = *desc
	cfg.App.Icon = *icon

	st, err := store.Default()
	if err != nil {
		return err
	}
	if st.Exists(cfg.App.Name) {
		return fmt.Errorf("app %q already exists at %s", cfg.App.Name, st.Path(cfg.App.Name))
	}
	if err := st.Save(cfg); err != nil { // validates first (image policy, schema, …)
		return err
	}
	fmt.Printf("created %s (%s preset) → %s\n", cfg.App.Name, *preset, st.Path(cfg.App.Name))
	return nil
}

func cmdList() error {
	st, err := store.Default()
	if err != nil {
		return err
	}
	names, err := st.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("no apps defined yet — create one with: hzp new <name> --image <img>")
		return nil
	}
	for _, n := range names {
		cfg, err := st.Load(n)
		if err != nil {
			fmt.Printf("%-20s (error: %v)\n", n, err)
			continue
		}
		fmt.Printf("%-20s %-12s %s\n", n, presetLabel(cfg.App.Preset), cfg.App.Image)
	}
	return nil
}

func cmdValidate(argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: hzp validate <name|app.toml>")
	}
	cfg, err := loadApp(argv[0])
	if err != nil {
		return err
	}
	if verr := config.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], verr)
	}
	fmt.Printf("ok: %s — image=%s preset=%s network=%s\n",
		cfg.App.Name, cfg.App.Image, presetLabel(cfg.App.Preset), cfg.Network.Mode)
	return nil
}

func cmdDelete(argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: hzp delete <name>")
	}
	st, err := store.Default()
	if err != nil {
		return err
	}
	if !st.Exists(argv[0]) {
		return fmt.Errorf("no app %q defined", argv[0])
	}
	if err := st.Delete(argv[0]); err != nil {
		return err
	}
	fmt.Printf("deleted %s\n", argv[0])
	return nil
}

func cmdRun(argv []string) error {
	if len(argv) < 1 {
		return fmt.Errorf("usage: hzp run <name|app.toml> [--exec]")
	}
	cfg, err := loadApp(argv[0])
	if err != nil {
		return err
	}
	if verr := config.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], verr)
	}
	args, err := runspec.BuildArgs(cfg, optionsFromEnv())
	if err != nil {
		return err
	}
	if !contains(argv[1:], "--exec") {
		fmt.Println("podman " + strings.Join(quoteForDisplay(args), " "))
		return nil
	}
	return podman(args...)
}

func cmdLifecycle(cmd string, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: hzp %s <name>", cmd)
	}
	name := argv[0]
	switch cmd {
	case "stop":
		return podman(runspec.StopArgs(name)...)
	case "restart":
		return podman(runspec.RestartArgs(name)...)
	case "inspect":
		return podman(runspec.InspectArgs(name)...)
	}
	return fmt.Errorf("unreachable: %q", cmd)
}

func cmdLogs(argv []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fs.Bool("f", false, "follow log output")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: hzp logs <name> [-f]")
	}
	return podman(runspec.LogsArgs(fs.Arg(0), *follow)...)
}

// loadApp resolves an app by store name or by file path. An argument containing a
// path separator or ending in ".toml" is read directly; otherwise it is looked up
// in the store.
func loadApp(arg string) (config.AppConfig, error) {
	if strings.Contains(arg, "/") || strings.HasSuffix(arg, ".toml") {
		return config.Load(arg)
	}
	st, err := store.Default()
	if err != nil {
		return config.AppConfig{}, err
	}
	if !st.Exists(arg) {
		return config.AppConfig{}, fmt.Errorf("no app %q defined (try: hzp list)", arg)
	}
	return st.Load(arg)
}

// podman runs the podman CLI with the host's stdio attached.
func podman(args ...string) error {
	pc := exec.Command("podman", args...)
	pc.Stdin, pc.Stdout, pc.Stderr = os.Stdin, os.Stdout, os.Stderr
	return pc.Run()
}

func optionsFromEnv() runspec.Options {
	return runspec.Options{
		RuntimeDir:     os.Getenv("XDG_RUNTIME_DIR"),
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		ThemeBundleDir: os.Getenv("HYPRZINC_THEME_BUNDLE"),
		HomeDir:        "/root",
	}
}

func presetLabel(p string) string {
	if p == "" {
		return "(none)"
	}
	return p
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// quoteForDisplay lightly quotes args with whitespace, for readable printing only.
func quoteForDisplay(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t") {
			out[i] = "'" + a + "'"
		} else {
			out[i] = a
		}
	}
	return out
}

// Command zcc is Zinc's app-definition tool (docs/architecture.md §9.1).
//
// It authors app files (~/.config/zinc/apps/<name>.yaml) and manages them: create, edit,
// list, validate, delete, and a keyboard-first TUI. To RUN what it authors it shells out
// to the `zcr` binary — Zinc's container runtime — so zcc never imports the runner and
// knows nothing about podman; the two meet only at the on-disk format and at that process
// boundary. zcr must be on $PATH for the run/manage commands; authoring works without it.
//
//	zcc tui                             keyboard-first manager (create/edit/run/stop/logs)
//	zcc new <name> --image <img> [--desc d] [--icon i]
//	zcc list
//	zcc validate <name|app.yaml>
//	zcc delete <name>
//	zcc keys list|show|set <s>|edit|validate|path   TUI keybind schemes
//	zcc run <name|app.yaml> [--exec]    ⟶ zcr run
//	zcc build <name|app.yaml>           ⟶ zcr build
//	zcc stop|restart|inspect <name>     ⟶ zcr
//	zcc logs <name> [-f]                ⟶ zcr logs
//	zcc term <name> [--shell]           ⟶ zcr term
//	zcc image search <term>|resolve <ref>   ⟶ zcr image
//
// A bare <name> resolves against the store (~/.config/zinc/apps); an argument that looks
// like a path (contains "/" or ends in ".yaml") is read directly.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/creator/internal/backend"
	"github.com/crispuscrew/zinc/container/creator/internal/keys"
	"github.com/crispuscrew/zinc/container/creator/internal/runner"
	"github.com/crispuscrew/zinc/container/creator/internal/store"
	"github.com/crispuscrew/zinc/container/creator/internal/tui"
)

const usage = `usage: zcc <command> [args]

  tui                               keyboard-first manager (create/edit/run/stop/logs)
  new <name> --image <img> [--desc d] [--icon i]
  list
  validate <name|app.yaml>
  delete <name>
  keys list|show|set <s>|edit|validate|path   TUI keybind schemes (default|vim|custom)
  run <name|app.yaml> [--exec]      build the launch plan; print it, or launch    (⟶ zcr)
  build <name|app.yaml>             (re)build the derived image (ImageMeta.Install) (⟶ zcr)
  stop|restart|inspect <name>       (⟶ zcr)
  logs <name> [-f]                  (⟶ zcr)
  term <name> [--shell]             open a terminal for a multiterminal app        (⟶ zcr)
  image search <term>|resolve <ref> find/pin an image (§5.5)                       (⟶ zcr)`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zcc: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) < 1 {
		return fmt.Errorf("%s", usage)
	}
	cmd, rest := argv[0], argv[1:]

	// keys is self-contained (zcc's own config dir); dispatch it before building the
	// store — it needs neither the store nor the runtime.
	if cmd == "keys" {
		return cmdKeys(rest)
	}

	// Runtime commands are delegated verbatim to the zcr binary (Zinc's runtime): zcc
	// authors app files, zcr runs them. Forwarding the whole argv keeps `zcc run X`
	// identical to `zcr run X`, streaming output live and preserving the exit status.
	switch cmd {
	case "run", "build", "stop", "restart", "inspect", "logs", "term", "image":
		return runner.Passthrough(argv...)
	}

	// Authoring commands work on the store locally; no runtime needed.
	sto, err := store.Default()
	if err != nil {
		return err
	}
	svc := backend.New(sto)

	switch cmd {
	case "tui":
		return cmdTUI(svc)
	case "new":
		return cmdNew(svc, rest)
	case "list":
		return cmdList(svc)
	case "validate":
		return cmdValidate(svc, rest)
	case "delete":
		return cmdDelete(svc, rest)
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

func cmdTUI(svc backend.Service) error {
	_, err := tea.NewProgram(tui.New(svc, loadKeys()), tea.WithAltScreen()).Run()
	return err
}

// loadKeys resolves the active TUI keybind scheme. A missing or broken scheme must never
// stop the TUI from starting, so any error falls back to the default (today's bindings)
// with a warning on stderr.
func loadKeys() keys.Active {
	if kst, err := keys.DefaultStore(); err == nil {
		if active, lerr := kst.Load(); lerr == nil {
			return active
		} else {
			fmt.Fprintln(os.Stderr, "zcc: keybinds: "+lerr.Error()+" — using default")
		}
	} else {
		fmt.Fprintln(os.Stderr, "zcc: keybinds: "+err.Error()+" — using default")
	}
	return keys.Active{Name: "default", Scheme: keys.Default}
}

func cmdNew(svc backend.Service, argv []string) error {
	// The name is the first argument; flags follow it (Go's flag parser stops at the
	// first positional, so "new <name> --image …" must split this way).
	if len(argv) < 1 || strings.HasPrefix(argv[0], "-") {
		return fmt.Errorf("usage: zcc new <name> --image <img> [--desc d] [--icon i]")
	}
	name, flags := argv[0], argv[1:]

	fset := flag.NewFlagSet("new", flag.ContinueOnError)
	image := fset.String("image", "", "container image (digest-pinned for third-party; §5.5)")
	desc := fset.String("desc", "", "human-readable description")
	icon := fset.String("icon", "", "icon name")
	if err := fset.Parse(flags); err != nil {
		return err
	}
	if fset.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q (flags must follow the name)", fset.Arg(0))
	}

	// Seed a minimal container definition; validate.Validate (via Save) enforces the rest
	// (image policy, schema). The user fleshes it out with `zcc tui` or a hand edit.
	cfg := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     name,
		Description:   *desc,
		Icon:          *icon,
	}
	cfg.ImageMeta.Image = *image

	if svc.Exists(cfg.AppNameID) {
		return fmt.Errorf("app %q already exists at %s", cfg.AppNameID, svc.Path(cfg.AppNameID))
	}
	if err := svc.Save(cfg); err != nil { // validates first (image policy, schema, …)
		return err
	}
	fmt.Printf("created %s → %s\n", cfg.AppNameID, svc.Path(cfg.AppNameID))
	return nil
}

func cmdList(svc backend.Service) error {
	names, err := svc.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("no apps defined yet — create one with: zcc new <name> --image <img>")
		return nil
	}
	for _, name := range names {
		cfg, err := svc.Load(name)
		if err != nil {
			fmt.Printf("%-20s (error: %v)\n", name, err)
			continue
		}
		fmt.Printf("%-20s %-10s %s\n", name, netLabel(cfg), cfg.ImageMeta.Image)
	}
	return nil
}

func cmdValidate(svc backend.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcc validate <name|app.yaml>")
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	if verr := validate.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], verr)
	}
	fmt.Printf("ok: %s — image=%s network=%s\n", cfg.AppNameID, cfg.ImageMeta.Image, netLabel(cfg))
	for _, warn := range validate.Warnings(cfg) {
		fmt.Println("warning: " + warn)
	}
	return nil
}

func cmdDelete(svc backend.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcc delete <name>")
	}
	if !svc.Exists(argv[0]) {
		return fmt.Errorf("no app %q defined", argv[0])
	}
	if err := svc.Delete(argv[0]); err != nil {
		return err
	}
	fmt.Printf("deleted %s\n", argv[0])
	return nil
}

// cmdKeys manages zcc's TUI keybind schemes (§9.1): list the available schemes, show a
// scheme's effective bindings, set the active one, edit/scaffold a custom scheme,
// validate, or print the config dir. These are zcc's own UI keys — not the desktop
// hotkeys (§12).
func cmdKeys(argv []string) error {
	kst, err := keys.DefaultStore()
	if err != nil {
		return err
	}
	sub := "list"
	if len(argv) > 0 {
		sub = argv[0]
	}
	switch sub {
	case "list":
		active, _ := kst.Load()
		names, err := kst.List()
		if err != nil {
			return err
		}
		for _, name := range names {
			mark := "  "
			if name == active.Name {
				mark = "* "
			}
			kind := "custom"
			if keys.IsBuiltin(name) {
				kind = "built-in"
			}
			fmt.Printf("%s%-20s %s\n", mark, name, kind)
		}
		return nil
	case "show":
		if len(argv) > 1 {
			scheme, err := kst.Resolve(argv[1])
			if err != nil {
				return err
			}
			return printScheme(argv[1], scheme)
		}
		active, err := kst.Load()
		if err != nil {
			return err
		}
		return printScheme(active.Name, active.Scheme)
	case "set":
		if len(argv) != 2 {
			return fmt.Errorf("usage: zcc keys set <scheme>")
		}
		if err := kst.SetActive(argv[1]); err != nil {
			return err
		}
		fmt.Printf("active keybind scheme: %s\n", argv[1])
		return nil
	case "edit":
		name := "default"
		if len(argv) > 1 {
			name = argv[1]
		}
		scheme, path, err := kst.EnsureEditable(name)
		if err != nil {
			return err
		}
		if err := openInEditor(path); err != nil {
			return err
		}
		if verr := kst.Validate(scheme); verr != nil {
			return fmt.Errorf("scheme %q has problems:\n%w", scheme, verr)
		}
		fmt.Printf("saved scheme %q (%s)\n  activate it with: zcc keys set %s\n", scheme, path, scheme)
		return nil
	case "validate":
		if len(argv) > 1 {
			if err := kst.Validate(argv[1]); err != nil {
				return err
			}
			fmt.Printf("ok: scheme %q is valid\n", argv[1])
			return nil
		}
		active, err := kst.Load() // Load resolves + validates the active scheme
		if err != nil {
			return err
		}
		fmt.Printf("ok: active scheme %q is valid\n", active.Name)
		return nil
	case "path":
		fmt.Println(kst.Dir)
		return nil
	default:
		return fmt.Errorf("unknown keys subcommand %q (want list|show|set|edit|validate|path)", sub)
	}
}

// printScheme prints a scheme's bindings grouped by screen, in action order.
func printScheme(name string, scheme keys.Scheme) error {
	fmt.Printf("scheme %q\n", name)
	for _, ctx := range keys.Contexts {
		fmt.Printf("  [%s]\n", keys.ContextName[ctx])
		for _, act := range keys.ActionsByContext[ctx] {
			if hint := scheme.Hint(ctx, act); hint != "" {
				fmt.Printf("    %-14s %s\n", act, hint)
			}
		}
	}
	return nil
}

// openInEditor opens path in $EDITOR (default vim) with the host's stdio.
func openInEditor(path string) error {
	argv := strings.Fields(os.Getenv("EDITOR"))
	if len(argv) == 0 {
		argv = []string{"vim"}
	}
	argv = append(argv, path)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// loadApp resolves an app by store name or by file path. An argument containing a path
// separator or ending in ".yaml" is read directly; otherwise it is looked up in the store.
func loadApp(svc backend.Service, arg string) (schema.AppConfig, error) {
	if strings.Contains(arg, "/") || strings.HasSuffix(arg, ".yaml") {
		return svc.LoadFile(arg)
	}
	if !svc.Exists(arg) {
		return schema.AppConfig{}, fmt.Errorf("no app %q defined (try: zcc list)", arg)
	}
	return svc.Load(arg)
}

// netLabel summarizes an app's network posture for the list/validate output: "isolated"
// when it has no NetworkLists (own localhost only), else the number of lists it carries.
func netLabel(cfg schema.AppConfig) string {
	if n := len(cfg.NetworkMeta.NetworkLists); n > 0 {
		return fmt.Sprintf("net:%d", n)
	}
	return "isolated"
}

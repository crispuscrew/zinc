// Command hzc is HyprZinc's container-definition tool (docs/architecture.md §9.1).
//
// It is the composition root of the hexagon: it assembles the core application
// service (core/wire → store + podman runtime/builder/resolver + the egress
// enforcers) and drives it from the CLI and the Bubbletea TUI.
//
//	hzc tui                           keyboard-first manager
//	hzc new <name> --image <img> [--preset strict|standard|networked]
//	hzc list                          # defined apps
//	hzc validate <name|app.toml>      # parse + validate, report problems
//	hzc delete <name>
//	hzc run <name|app.toml> [--exec]  # build the launch plan; print it, or launch
//	hzc build <name|app.toml>         # (re)build an app's derived image (app.install)
//	hzc stop|restart|inspect <name>
//	hzc logs <name> [-f]
//
// A bare <name> resolves against the store (~/.config/hyprzinc/apps); an argument
// that looks like a path (contains "/" or ends in ".toml") is read directly.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crispuscrew/hyprzinc/core/adapters/host"
	"github.com/crispuscrew/hyprzinc/core/adapters/podman"
	"github.com/crispuscrew/hyprzinc/core/app"
	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/core/ports"
	"github.com/crispuscrew/hyprzinc/core/wire"
	"github.com/crispuscrew/hyprzinc/hzc/internal/keys"
	"github.com/crispuscrew/hyprzinc/hzc/internal/tui"
)

const usage = `usage: hzc <command> [args]

  tui                               keyboard-first manager (create/edit/run/stop/logs)
  new <name> --image <img> [--preset strict|standard|networked] [--desc d] [--icon i]
  list
  validate <name|app.toml>
  delete <name>
  run <name|app.toml> [--exec]
  build <name|app.toml>            (re)build the derived image for an app with app.install (§5.5)
  image search <term>              find images on configured registries
  image resolve <ref>              pin a tag to its @sha256 digest (§5.5)
  keys list|show|set <s>|edit|validate|path   TUI keybind schemes (default|vim|custom)
  term <name> [--shell]            open a terminal for a multiterminal app (§9.1)
  stop|restart|inspect <name>
  logs <name> [-f]`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hzc: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) < 1 {
		return fmt.Errorf("%s", usage)
	}
	// keys is self-contained (hzc's own config dir), so dispatch it before wiring the
	// app service — it needs neither the store nor podman.
	cmd, rest := argv[0], argv[1:]
	if cmd == "keys" {
		return cmdKeys(rest)
	}

	svc, err := wire.DefaultService()
	if err != nil {
		return err
	}
	opt := host.Options()

	switch cmd {
	case "tui":
		return cmdTUI(svc, opt)
	case "new":
		return cmdNew(svc, rest)
	case "list":
		return cmdList(svc)
	case "validate":
		return cmdValidate(svc, rest)
	case "delete":
		return cmdDelete(svc, rest)
	case "run":
		return cmdRun(svc, opt, rest)
	case "build":
		return cmdBuild(svc, rest)
	case "image":
		return cmdImage(svc, rest)
	case "term":
		return cmdTerm(svc, opt, rest)
	case "__term":
		// Hidden: the per-terminal waiter process spawned by OpenTerminal. It blocks
		// until the terminal closes and is responsible for the last-one-out stop.
		return cmdTermWaiter(svc, opt, rest)
	case "stop", "restart", "inspect":
		return cmdLifecycle(svc, opt, cmd, rest)
	case "logs":
		return cmdLogs(svc, rest)
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

func cmdTUI(svc app.Service, opt domain.HostOptions) error {
	_, err := tea.NewProgram(tui.New(svc, opt, loadKeys()), tea.WithAltScreen()).Run()
	return err
}

// loadKeys resolves the active TUI keybind scheme. A missing or broken scheme must
// never stop the TUI from starting, so any error falls back to the default (today's
// bindings) with a warning on stderr.
func loadKeys() keys.Active {
	if kst, err := keys.DefaultStore(); err == nil {
		if active, lerr := kst.Load(); lerr == nil {
			return active
		} else {
			fmt.Fprintln(os.Stderr, "hzc: keybinds: "+lerr.Error()+" — using default")
		}
	} else {
		fmt.Fprintln(os.Stderr, "hzc: keybinds: "+err.Error()+" — using default")
	}
	return keys.Active{Name: "default", Scheme: keys.Default}
}

func cmdNew(svc app.Service, argv []string) error {
	// The name is the first argument; flags follow it (Go's flag parser stops at the
	// first positional, so "new <name> --image …" must split this way).
	if len(argv) < 1 || strings.HasPrefix(argv[0], "-") {
		return fmt.Errorf("usage: hzc new <name> --image <img> [--preset strict|standard|networked]")
	}
	name, flags := argv[0], argv[1:]

	fset := flag.NewFlagSet("new", flag.ContinueOnError)
	image := fset.String("image", "", "container image (digest-pinned for third-party; §5.5)")
	preset := fset.String("preset", domain.PresetDefaultNew, "preset template: strict|standard|networked")
	desc := fset.String("desc", "", "human-readable description")
	icon := fset.String("icon", "", "icon name")
	if err := fset.Parse(flags); err != nil {
		return err
	}
	if fset.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q (flags must follow the name)", fset.Arg(0))
	}
	cfg, ok := domain.DefaultsFor(*preset)
	if !ok {
		return fmt.Errorf("unknown preset %q (want strict|standard|networked)", *preset)
	}
	cfg.App.Name = name
	cfg.App.Image = *image
	cfg.App.Description = *desc
	cfg.App.Icon = *icon

	if svc.Exists(cfg.App.Name) {
		return fmt.Errorf("app %q already exists at %s", cfg.App.Name, svc.Path(cfg.App.Name))
	}
	if err := svc.Save(cfg); err != nil { // validates first (image policy, schema, …)
		return err
	}
	fmt.Printf("created %s (%s preset) → %s\n", cfg.App.Name, *preset, svc.Path(cfg.App.Name))
	return nil
}

func cmdList(svc app.Service) error {
	names, err := svc.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("no apps defined yet — create one with: hzc new <name> --image <img>")
		return nil
	}
	for _, name := range names {
		cfg, err := svc.Load(name)
		if err != nil {
			fmt.Printf("%-20s (error: %v)\n", name, err)
			continue
		}
		fmt.Printf("%-20s %-12s %s\n", name, presetLabel(cfg.App.Preset), cfg.App.Image)
	}
	return nil
}

func cmdValidate(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: hzc validate <name|app.toml>")
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	if verr := domain.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], verr)
	}
	fmt.Printf("ok: %s — image=%s preset=%s network=%s\n",
		cfg.App.Name, cfg.App.Image, presetLabel(cfg.App.Preset), cfg.Network.Mode)
	return nil
}

func cmdDelete(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: hzc delete <name>")
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

func cmdRun(svc app.Service, opt domain.HostOptions, argv []string) error {
	if len(argv) < 1 {
		return fmt.Errorf("usage: hzc run <name|app.toml> [--exec]")
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	if contains(argv[1:], "--exec") {
		// Launch through the shared service (validate → build derived image → lock down
		// → detach) — the same path the TUI and hzl use.
		return svc.Launch(cfg, opt)
	}
	// Dry-run: validate and print the exact podman command(s) without running them.
	if verr := domain.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], verr)
	}
	if domain.HasInstall(cfg) {
		// The app runs a derived image (FROM app.image + the install layer); a real run
		// builds it first (auto on change, or `hzc build`). Show it so the plan matches.
		fmt.Println("# build derived image first (auto on run when stale, or: hzc build " + cfg.App.Name + ")")
		fmt.Println("podman " + strings.Join(quoteForDisplay(podman.ImageBuildArgs(cfg)), " "))
		fmt.Print(domain.DerivedContainerfile(cfg))
	}
	for _, line := range runAdvisories(cfg) {
		fmt.Println(line)
	}
	plan, err := svc.Plan(cfg, opt)
	if err != nil {
		return err
	}
	printPlan(plan)
	if cfg.App.Multiterminal {
		// The plan above starts the shared holder; each terminal attaches with this
		// exec (run `hzc term <name>` to open one, `--shell` for a shell).
		fmt.Println("# each terminal attaches to the holder")
		fmt.Println("podman " + strings.Join(quoteForDisplay(podman.ExecArgs(cfg.App.Name, cfg.App.Command)), " "))
	}
	return nil
}

// runAdvisories returns `#`-prefixed warnings for config that validation allows but a
// reviewer should see before launching — capabilities re-added past the cap-drop all
// baseline, an effectively-open pasta allowlist, and host device exposure (GPU/ALSA).
// Dry-run preview only; the plan itself is unchanged.
func runAdvisories(cfg domain.AppConfig) []string {
	var lines []string
	if len(cfg.Capabilities.Extra) > 0 {
		lines = append(lines, "# WARNING: capabilities.extra re-adds capabilities beyond the `cap-drop all` baseline: "+strings.Join(cfg.Capabilities.Extra, ", "))
	}
	if cfg.Network.Mode == domain.NetworkPasta {
		if contains(cfg.Network.IPv4CIDR, "0.0.0.0/0") || contains(cfg.Network.IPv6CIDR, "::/0") {
			lines = append(lines, "# WARNING: pasta egress allowlist includes an all-egress CIDR (0.0.0.0/0 or ::/0) — the allowlist is effectively open")
		}
	}
	if cfg.Display.GPU {
		lines = append(lines, "# WARNING: display.gpu exposes the host GPU (/dev/dri) to the container")
	}
	if cfg.Audio.LegacyALSA {
		lines = append(lines, "# WARNING: audio.legacy_alsa exposes the host sound device (/dev/snd) to the container")
	}
	return lines
}

// cmdBuild (re)builds an app's derived image (FROM app.image + the app.install
// layer). A plain `hzc run` already rebuilds on demand when the install line or base
// changes; this is the explicit build, e.g. to pre-warm it or retry after fixing a
// failing install line. Build output is captured and surfaced on failure (§5.5, §9.1).
func cmdBuild(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: hzc build <name|app.toml>")
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	if verr := domain.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], verr)
	}
	if !domain.HasInstall(cfg) {
		return fmt.Errorf("%s: no app.install set — nothing to build; it runs %s directly", cfg.App.Name, cfg.App.Image)
	}
	fmt.Printf("# building %s (FROM %s)\n", domain.DerivedImageRef(cfg.App.Name), cfg.App.Image)
	if err := svc.Build(cfg); err != nil {
		return err
	}
	fmt.Printf("built %s\n", domain.DerivedImageRef(cfg.App.Name))
	return nil
}

// parseTermArgs splits `<name> [--shell]` shared by `term` and the hidden `__term`.
func parseTermArgs(argv []string) (name string, shell bool, err error) {
	for _, arg := range argv {
		switch {
		case arg == "--shell":
			shell = true
		case strings.HasPrefix(arg, "-"):
			return "", false, fmt.Errorf("unknown flag %q\nusage: hzc term <name> [--shell]", arg)
		case name == "":
			name = arg
		default:
			return "", false, fmt.Errorf("unexpected argument %q\nusage: hzc term <name> [--shell]", arg)
		}
	}
	if name == "" {
		return "", false, fmt.Errorf("usage: hzc term <name> [--shell]")
	}
	return name, shell, nil
}

// cmdTerm opens one more terminal for a multiterminal app: it spawns a detached
// waiter and returns. The first terminal starts the shared holder; the holder lives
// until the last terminal closes, unless the app is background (§9.1).
func cmdTerm(svc app.Service, opt domain.HostOptions, argv []string) error {
	name, shell, err := parseTermArgs(argv)
	if err != nil {
		return err
	}
	cfg, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	return svc.OpenTerminal(cfg, opt, shell)
}

// cmdTermWaiter is the hidden `__term` waiter: it runs (and blocks in) the spawned
// process, opening one terminal and stopping the holder if it is the last to close.
func cmdTermWaiter(svc app.Service, opt domain.HostOptions, argv []string) error {
	name, shell, err := parseTermArgs(argv)
	if err != nil {
		return err
	}
	cfg, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	return svc.Term(cfg, opt, shell)
}

// cmdImage helps choose an image without a browser: search registries, or resolve a
// tag to its digest-pinned form (§5.5) ready to paste into app.image.
func cmdImage(svc app.Service, argv []string) error {
	if len(argv) != 2 {
		return fmt.Errorf("usage: hzc image search <term> | hzc image resolve <ref>")
	}
	sub, arg := argv[0], argv[1]
	switch sub {
	case "search":
		results, err := svc.Search(arg)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			fmt.Println("no images found")
			return nil
		}
		for _, result := range results {
			fmt.Printf("%s\t%s\n", result.Name, result.Description)
		}
		return nil
	case "resolve", "pin":
		pinned, err := svc.Resolve(arg)
		if err != nil {
			return err
		}
		fmt.Println(pinned)
		return nil
	default:
		return fmt.Errorf("unknown image subcommand %q (want search|resolve)", sub)
	}
}

// cmdKeys manages hzc's TUI keybind schemes (§9.1): list the available schemes, show
// a scheme's effective bindings, set the active one, edit/scaffold a custom scheme,
// validate, or print the config dir. These are hzc's own UI keys — not the Hyprland
// desktop hotkeys (§12).
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
			return fmt.Errorf("usage: hzc keys set <scheme>")
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
		fmt.Printf("saved scheme %q (%s)\n  activate it with: hzc keys set %s\n", scheme, path, scheme)
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

// printPlan shows the launch as the exact podman command(s). For a pasta app this is
// the three-step pod flow; the nft ruleset that gets piped in is printed too so what
// will be enforced is fully visible before anything runs.
func printPlan(plan []ports.Command) {
	for _, cmd := range plan {
		fmt.Println("# " + cmd.Desc)
		fmt.Println("podman " + strings.Join(quoteForDisplay(cmd.Args), " "))
		if cmd.Stdin != "" {
			fmt.Print(cmd.Stdin)
		}
	}
}

func cmdLifecycle(svc app.Service, opt domain.HostOptions, cmd string, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: hzc %s <name>", cmd)
	}
	name := argv[0]
	if cmd == "inspect" {
		return svc.Do(podman.InspectArgs(name))
	}
	// stop/restart must be pod-aware: a pasta app lives in a pod that owns its
	// filtered netns, so we load the definition to decide.
	cfg, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	switch cmd {
	case "stop":
		return svc.Stop(cfg)
	case "restart":
		if cfg.Network.Mode == domain.NetworkPasta {
			// nft rules live in the pod's netns and are lost on a plain pod restart, so
			// tear the pod down and relaunch through the service (re-applies them).
			_ = svc.Stop(cfg)
			return svc.Launch(cfg, opt)
		}
		return svc.Do(podman.RestartArgs(name))
	}
	return fmt.Errorf("unreachable: %q", cmd)
}

func cmdLogs(svc app.Service, argv []string) error {
	fset := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fset.Bool("f", false, "follow log output")
	if err := fset.Parse(argv); err != nil {
		return err
	}
	if fset.NArg() != 1 {
		return fmt.Errorf("usage: hzc logs <name> [-f]")
	}
	return svc.Do(podman.LogsArgs(fset.Arg(0), *follow))
}

// loadApp resolves an app by store name or by file path. An argument containing a
// path separator or ending in ".toml" is read directly; otherwise it is looked up in
// the store.
func loadApp(svc app.Service, arg string) (domain.AppConfig, error) {
	if strings.Contains(arg, "/") || strings.HasSuffix(arg, ".toml") {
		return svc.LoadFile(arg)
	}
	if !svc.Exists(arg) {
		return domain.AppConfig{}, fmt.Errorf("no app %q defined (try: hzc list)", arg)
	}
	return svc.Load(arg)
}

func presetLabel(preset string) string {
	if preset == "" {
		return "(none)"
	}
	return preset
}

func contains(list []string, want string) bool {
	for _, str := range list {
		if str == want {
			return true
		}
	}
	return false
}

// quoteForDisplay lightly quotes args with whitespace, for readable printing only.
func quoteForDisplay(args []string) []string {
	out := make([]string, len(args))
	for idx, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			out[idx] = "'" + arg + "'"
		} else {
			out[idx] = arg
		}
	}
	return out
}

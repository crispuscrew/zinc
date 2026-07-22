// Command zcr is Zinc's container runtime: it runs an app file
// (~/.config/zinc/apps/<name>.yaml) via podman, applying the egress lock-down before
// the app starts (docs/architecture.md section 5.3, section 9.1). It is the composition root of the
// runner hexagon: it assembles the app.Service from wire and drives it from the CLI.
//
//	zcr run <app> [--exec] [-v HOST:CONTAINER[:OPTIONS]]...
//	                            print the launch plan, or launch it (--exec); -v/--volume
//	                            adds a runtime-only bind mount (repeatable)
//	zcr build <app>             (re)build the app's derived image (ImageMeta.Install)
//	zcr validate <app>          parse + validate; report problems and warnings
//	zcr stop|restart|inspect <app>
//	zcr logs <app> [-f]
//	zcr term <app> [--shell]    open a terminal for a multiterminal app (section 9.1)
//	zcr ps                      running apps, one per line
//	zcr image search <term> | resolve <ref>
//
// <app> is a store name (~/.config/zinc/apps) or a path (contains '/' or ends .yaml).
// zcc (the creator) shells out to this binary to run what it authors.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/adapters/host"
	"github.com/crispuscrew/zinc/container/runner/adapters/podman"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
	"github.com/crispuscrew/zinc/container/runner/wire"
)

const usage = `usage: zcr <command> [args]

  run <app> [--exec] [-v HOST:CONTAINER[:OPTIONS]]...
                            print the launch plan, or launch it (--exec)
                            -v/--volume adds a runtime-only bind mount (repeatable;
                            OPTIONS default ro,noexec - use rw and/or exec)
  build <app>               (re)build the derived image (ImageMeta.Install)
  validate <app>            parse + validate; report problems and warnings
  stop|restart|inspect <app>
  logs <app> [-f]
  term <app> [--shell]      open a terminal for a multiterminal app
  ps                        running apps, one per line
  image search <term> | resolve <ref>
  version                   print the version

<app> is a store name (~/.config/zinc/apps) or a path (has '/' or ends in .yaml).`

// version is stamped at build time via -ldflags "-X main.version=..." (the Makefile
// derives it from `git describe`). It stays "dev" for a plain build.
var version = "dev"

// versionString returns the build-stamped version, falling back to the module version
// embedded by `go install <pkg>@vX` when ldflags did not set one.
func versionString() string {
	if version != "dev" && version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zcr: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) < 1 {
		return fmt.Errorf("%s", usage)
	}
	if argv[0] == "version" || argv[0] == "--version" {
		fmt.Println("zcr " + versionString())
		return nil
	}
	svc, err := wire.DefaultService()
	if err != nil {
		return err
	}
	opt := host.Options()

	cmd, rest := argv[0], argv[1:]
	switch cmd {
	case "run":
		return cmdRun(svc, opt, rest)
	case "build":
		return cmdBuild(svc, rest)
	case "validate":
		return cmdValidate(svc, rest)
	case "stop", "restart", "inspect":
		return cmdLifecycle(svc, opt, cmd, rest)
	case "logs":
		return cmdLogs(svc, rest)
	case "term":
		return cmdTerm(svc, opt, rest)
	case "__term":
		// Hidden: the per-terminal waiter spawned by OpenTerminal. It blocks until the
		// terminal closes and does the last-one-out stop.
		return cmdTermWaiter(svc, opt, rest)
	case "ps":
		return cmdPs(svc)
	case "image":
		return cmdImage(svc, rest)
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

func cmdRun(svc app.Service, opt options.HostOptions, argv []string) error {
	name, execute, runtimeVolumes, err := parseRunArgs(argv)
	if err != nil {
		return err
	}
	cfg, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	// Runtime-only volumes (-v/--volume): appended to the loaded config in memory and
	// never written back to the app YAML. Both branches below validate the whole config
	// before composing any podman arg, so these runtime mounts are screened by the same
	// checkVolume field-shift/injection guards as configured Volumes, and the existing
	// arg-builder mounts them (docs/architecture.md section 3).
	cfg.Volumes = append(cfg.Volumes, runtimeVolumes...)
	if execute {
		// Launch through the service: validate -> build derived image -> lock down -> detach.
		return svc.Launch(cfg, opt)
	}
	// Dry-run: validate and print the exact podman command(s) without running them.
	if verr := validate.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", name, verr)
	}
	if derived.HasInstall(cfg) {
		// The app runs a derived image (FROM ImageMeta.Image + the install layer); a real
		// run builds it first (auto on change, or `zcr build`). Show it so the plan matches.
		fmt.Println("# build derived image first (auto on run when stale, or: zcr build " + cfg.AppNameID + ")")
		fmt.Println("podman " + strings.Join(quoteForDisplay(podman.ImageBuildArgs(cfg)), " "))
		fmt.Print(derived.DerivedContainerfile(cfg))
	}
	for _, warn := range validate.Warnings(cfg) {
		fmt.Println("# WARNING: " + warn)
	}
	plan, err := svc.Plan(cfg, opt)
	if err != nil {
		return err
	}
	printPlan(plan)
	if cfg.StartConditions.Multiterminal {
		// The plan starts the shared holder; each terminal attaches with this exec
		// (run `zcr term <app>` to open one, `--shell` for a shell).
		fmt.Println("# each terminal attaches to the holder")
		fmt.Println("podman " + strings.Join(quoteForDisplay(podman.ExecArgs(cfg.AppNameID, termCmd(cfg))), " "))
	}
	return nil
}

// runUsage is the usage line for `zcr run`, shared by its argument errors.
const runUsage = "usage: zcr run <app> [--exec] [-v HOST:CONTAINER[:OPTIONS]]..."

// parseRunArgs splits `zcr run`'s arguments into the app name, the --exec flag, and any
// repeated -v/--volume runtime mounts. Flags may appear before or after the app name.
// Each volume value is HOST:CONTAINER[:OPTIONS] and is turned into an in-memory Volume
// by parseVolumeSpec; cmdRun appends these to the loaded config (validated there before
// use). The separated (-v VALUE) and attached (-v=VALUE, --volume=VALUE) forms are both
// accepted.
func parseRunArgs(argv []string) (name string, execute bool, volumes []schema.Volume, err error) {
	for idx := 0; idx < len(argv); idx++ {
		arg := argv[idx]
		switch {
		case arg == "--exec":
			execute = true
		case arg == "-v" || arg == "--volume":
			idx++
			if idx >= len(argv) {
				return "", false, nil, fmt.Errorf("%s: missing value (want HOST:CONTAINER[:OPTIONS])", arg)
			}
			vol, verr := parseVolumeSpec(argv[idx])
			if verr != nil {
				return "", false, nil, verr
			}
			volumes = append(volumes, vol)
		case strings.HasPrefix(arg, "-v="):
			vol, verr := parseVolumeSpec(strings.TrimPrefix(arg, "-v="))
			if verr != nil {
				return "", false, nil, verr
			}
			volumes = append(volumes, vol)
		case strings.HasPrefix(arg, "--volume="):
			vol, verr := parseVolumeSpec(strings.TrimPrefix(arg, "--volume="))
			if verr != nil {
				return "", false, nil, verr
			}
			volumes = append(volumes, vol)
		case strings.HasPrefix(arg, "-"):
			return "", false, nil, fmt.Errorf("unknown flag %q\n%s", arg, runUsage)
		case name == "":
			name = arg
		default:
			return "", false, nil, fmt.Errorf("unexpected argument %q\n%s", arg, runUsage)
		}
	}
	if name == "" {
		return "", false, nil, fmt.Errorf("%s", runUsage)
	}
	return name, execute, volumes, nil
}

// parseVolumeSpec parses one runtime -v/--volume value HOST:CONTAINER[:OPTIONS] into an
// in-memory host-mounted Volume. OPTIONS is a comma list with the same meaning as a
// configured volume's flags: the default is read-only and non-executable; "rw" makes it
// writable and "exec" executable ("ro"/"noexec" restate the defaults). HOST and
// CONTAINER must be non-empty; any ':'/','/whitespace they carry (a podman field-shift)
// is rejected by the config validation cmdRun runs before this Volume reaches podman.
func parseVolumeSpec(spec string) (schema.Volume, error) {
	fields := strings.Split(spec, ":")
	if len(fields) < 2 || len(fields) > 3 {
		return schema.Volume{}, fmt.Errorf("--volume %q: want HOST:CONTAINER[:OPTIONS]", spec)
	}
	host, inner := fields[0], fields[1]
	if strings.TrimSpace(host) == "" {
		return schema.Volume{}, fmt.Errorf("--volume %q: empty HOST path", spec)
	}
	if strings.TrimSpace(inner) == "" {
		return schema.Volume{}, fmt.Errorf("--volume %q: empty CONTAINER path", spec)
	}
	vol := schema.Volume{HostMounted: true, HostMount: host, InnerMount: inner}
	if len(fields) == 3 {
		for _, mountOpt := range strings.Split(fields[2], ",") {
			switch strings.TrimSpace(mountOpt) {
			case "rw":
				vol.Writable = true
			case "ro":
				vol.Writable = false
			case "exec":
				vol.Executable = true
			case "noexec":
				vol.Executable = false
			default:
				return schema.Volume{}, fmt.Errorf("--volume %q: unknown option %q (want rw, ro, exec, noexec)", spec, mountOpt)
			}
		}
	}
	return vol, nil
}

// termCmd is the argv each terminal of a multiterminal app runs: its
// MultiterminalEntrypoint, else its Entrypoint, split into fields.
func termCmd(cfg schema.AppConfig) []string {
	spec := cfg.StartConditions.MultiterminalEntrypoint
	if strings.TrimSpace(spec) == "" {
		spec = cfg.StartConditions.Entrypoint
	}
	return strings.Fields(spec)
}

// cmdBuild (re)builds an app's derived image (FROM ImageMeta.Image + ImageMeta.Install).
// A plain `zcr run` already rebuilds on demand when the install line or base changes;
// this is the explicit build (section 5.5, section 9.1).
func cmdBuild(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr build <app>")
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	if verr := validate.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], verr)
	}
	if !derived.HasInstall(cfg) {
		return fmt.Errorf("%s: no ImageMeta.Install set - nothing to build; it runs %s directly", cfg.AppNameID, cfg.ImageMeta.Image)
	}
	fmt.Printf("# building %s (FROM %s)\n", derived.DerivedImageRef(cfg.AppNameID), cfg.ImageMeta.Image)
	if err := svc.Build(cfg); err != nil {
		return err
	}
	fmt.Printf("built %s\n", derived.DerivedImageRef(cfg.AppNameID))
	return nil
}

func cmdValidate(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr validate <app>")
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	if verr := validate.Validate(cfg); verr != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], verr)
	}
	fmt.Printf("ok: %s - image=%s\n", cfg.AppNameID, cfg.ImageMeta.Image)
	for _, warn := range validate.Warnings(cfg) {
		fmt.Println("warning: " + warn)
	}
	return nil
}

func cmdLifecycle(svc app.Service, opt options.HostOptions, cmd string, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr %s <app>", cmd)
	}
	name := argv[0]
	if cmd == "inspect" {
		return svc.Do(podman.InspectArgs(name))
	}
	cfg, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	switch cmd {
	case "stop":
		return svc.Stop(cfg)
	case "restart":
		if len(cfg.NetworkMeta.NetworkLists) > 0 {
			// nft rules live in the pod's netns and are lost on a plain pod restart, so tear
			// the pod down and relaunch through the service (re-applies them).
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
		return fmt.Errorf("usage: zcr logs <app> [-f]")
	}
	return svc.Do(podman.LogsArgs(fset.Arg(0), *follow))
}

// parseTermArgs splits `<app> [--shell]`, shared by `term` and the hidden `__term`.
func parseTermArgs(argv []string) (name string, shell bool, err error) {
	for _, arg := range argv {
		switch {
		case arg == "--shell":
			shell = true
		case strings.HasPrefix(arg, "-"):
			return "", false, fmt.Errorf("unknown flag %q\nusage: zcr term <app> [--shell]", arg)
		case name == "":
			name = arg
		default:
			return "", false, fmt.Errorf("unexpected argument %q\nusage: zcr term <app> [--shell]", arg)
		}
	}
	if name == "" {
		return "", false, fmt.Errorf("usage: zcr term <app> [--shell]")
	}
	return name, shell, nil
}

// cmdTerm opens one more terminal for a multiterminal app: it spawns a detached waiter
// and returns. The first terminal starts the shared holder (section 9.1).
func cmdTerm(svc app.Service, opt options.HostOptions, argv []string) error {
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

// cmdTermWaiter is the hidden `__term` waiter: it opens one terminal and stops the
// holder if it is the last to close.
func cmdTermWaiter(svc app.Service, opt options.HostOptions, argv []string) error {
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

// cmdPs prints the apps podman reports as running, one per line and sorted, so a
// front-end (zcc) can read it to show live state.
func cmdPs(svc app.Service) error {
	running, err := svc.Running()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(running))
	for name, up := range running {
		if up {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Println(name)
	}
	return nil
}

// cmdImage helps choose an image without a browser: search registries, or resolve a tag
// to its digest-pinned form (section 5.5) ready to paste into ImageMeta.Image.
func cmdImage(svc app.Service, argv []string) error {
	if len(argv) != 2 {
		return fmt.Errorf("usage: zcr image search <term> | zcr image resolve <ref>")
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

// printPlan shows the launch as the exact podman command(s). For a filtered app this is
// the multi-step pod flow; the nft ruleset piped in is printed too, so what will be
// enforced is fully visible before anything runs.
func printPlan(plan []ports.Command) {
	for _, cmd := range plan {
		fmt.Println("# " + cmd.Desc)
		fmt.Println("podman " + strings.Join(quoteForDisplay(cmd.Args), " "))
		if cmd.Stdin != "" {
			fmt.Print(cmd.Stdin)
		}
	}
}

// loadApp resolves an app by store name or by file path. An argument containing a path
// separator or ending in ".yaml" is read directly; otherwise it is looked up in the store.
func loadApp(svc app.Service, arg string) (schema.AppConfig, error) {
	if strings.Contains(arg, "/") || strings.HasSuffix(arg, ".yaml") {
		return svc.LoadFile(arg)
	}
	if !svc.Exists(arg) {
		return schema.AppConfig{}, fmt.Errorf("no app %q defined (try: zcc list)", arg)
	}
	return svc.Load(arg)
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

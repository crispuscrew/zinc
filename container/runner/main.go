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
// zc (the creator) shells out to this binary to run what it authors.
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
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
	"github.com/crispuscrew/zinc/container/runner/ports"
	"github.com/crispuscrew/zinc/container/runner/wire"
)

const usage = `usage: zcr <command> [args]

  run <app[@instance]> [--instance NAME] [--exec] [-v HOST:CONTAINER[:OPTIONS]]...
                            print the launch plan, or launch it (--exec)
                            -v/--volume adds a runtime-only bind mount (repeatable;
                            OPTIONS default ro,noexec - use rw and/or exec)
  build <app>               (re)build the derived image (ImageMeta.Install)
  validate <app>            parse + validate; report problems and warnings
  stop|restart|inspect <app>
  logs <app> [-f]
  term <app> [--shell]      open a terminal for a multiterminal app
  ps                        running apps, one per line
  where <app[@instance]>    print where that instance keeps its state, and the name
                            its container takes; ask rather than assume the layout
  image search <term> | resolve <ref>
  version                   print the version

<app> is a store name (~/.config/zinc/apps) or a path (has '/' or ends in .yaml).`

// cmdWhere answers "where does this instance keep things, and what is it called at runtime".
//
// It exists so nothing outside Zinc has to hardcode the layout. A desktop that wants to show
// a user where an app's state lives, or that names a container to look it up, would otherwise
// mirror the rules in paths - and two copies of a layout drift the first time either side
// changes. Asking costs a process; assuming costs a bug nobody sees until the paths differ.
//
// Deliberately not folded into `inspect`, which is a passthrough to `podman inspect`:
// intercepting it would put Zinc in the business of parsing and re-emitting podman's output
// forever, and the answer here is about an instance whether or not it is running.
//
// The output is two labelled lines rather than JSON because it is also read by people. A
// consumer that wants one value cuts on the colon; the labels are the contract.
func cmdWhere(argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr where <app[@instance]>")
	}
	addr, err := paths.ParseAddress(argv[0])
	if err != nil {
		return err
	}
	if strings.TrimSpace(addr.App) == "" {
		return fmt.Errorf("usage: zcr where <app[@instance]>")
	}
	stateDir, err := paths.StateDir(addr)
	if err != nil {
		return err
	}
	fmt.Printf("state: %s\n", stateDir)
	fmt.Printf("container: %s\n", addr.Runtime())
	return nil
}

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
	case "where":
		return cmdWhere(rest)
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
	// --instance is the flag form of the "app@instance" address. Both exist because they are
	// convenient in different places - a flag reads better in a hand-typed command, an address
	// travels better through a script that already has one string - and they fold into the same
	// address here so nothing downstream has to know which was used.
	var instance string
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
		case arg == "--instance":
			idx++
			if idx >= len(argv) {
				return "", false, nil, fmt.Errorf("--instance: missing value (want a name like 'work')")
			}
			instance = argv[idx]
		case strings.HasPrefix(arg, "--instance="):
			instance = strings.TrimPrefix(arg, "--instance=")
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
	if instance != "" {
		if strings.Contains(name, "@") {
			return "", false, nil, fmt.Errorf("%q already names an instance, so --instance would have to override it; give one or the other", name)
		}
		name += "@" + instance
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
		if err := refuseVM(svc, name); err != nil {
			return err
		}
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
	if err := refuseVM(svc, fset.Arg(0)); err != nil {
		return err
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
// front-end (zc) can read it to show live state.
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
// Every command that needs a config goes through here, which is why the app-type check
// lives here: one store holds both container and VM apps, and zcr runs only the
// containers. The passthrough commands that never load a config (inspect, logs) call
// refuseVM instead.
func loadApp(svc app.Service, arg string) (schema.AppConfig, error) {
	cfg, err := load(svc, arg)
	if err != nil {
		return schema.AppConfig{}, err
	}
	if cfg.Type == schema.ZincVirtualization {
		// Refuse rather than try: none of what follows - the image build, the pod, the
		// nftables lock-down - means anything for a guest, and a half-applied container
		// launch is exactly the mis-enforcement the network model refuses elsewhere.
		return schema.AppConfig{}, fmt.Errorf("app %q is a VM app (Type: %s); run it with zvr", cfg.AppNameID, cfg.Type)
	}
	return cfg, nil
}

// refuseVM stops a container-only passthrough from being aimed at a VM app. inspect and
// logs hand the name straight to podman without loading anything, so without this they
// fail with podman's "no such object" - true, but silent about the actual reason, which
// is that the app is a guest and zvr owns it. A name that is not a defined app is left
// alone: it may legitimately be a raw container name.
func refuseVM(svc app.Service, name string) error {
	cfg, err := load(svc, name)
	if err != nil || cfg.Type != schema.ZincVirtualization {
		// Not a defined app (it may legitimately be a raw container name), or unreadable,
		// or a container: either way this guard has nothing to say and podman answers.
		return nil
	}
	return fmt.Errorf("app %q is a VM app (Type: %s); use zvr", name, cfg.Type)
}

// load reads the app a command names, with its Inherits chain applied: the runner acts on
// what an app IS, not on the part of it that happens to be written in its own file.
func load(svc app.Service, arg string) (schema.AppConfig, error) {
	if strings.Contains(arg, "/") || strings.HasSuffix(arg, ".yaml") {
		// A path names a file, and a file is one definition. Instances address the store,
		// where a name can be run more than once.
		return svc.LoadFileResolved(arg)
	}
	addr, err := paths.ParseAddress(arg)
	if err != nil {
		return schema.AppConfig{}, err
	}
	if !svc.Exists(addr.App) {
		return schema.AppConfig{}, fmt.Errorf("no app %q defined (try: zc list)", addr.App)
	}
	cfg, err := svc.LoadResolved(addr.App)
	if err != nil {
		return schema.AppConfig{}, err
	}
	// The instance rides on AppNameID from here down, because AppNameID is what every
	// runtime name already derives from: the pod, the app container, the healthcheck the
	// dependents wait on, the D-Bus proxy and its socket directory, and everything teardown
	// removes. Rewriting it once here is what makes an instance a first-class running thing
	// without threading a second identifier through every adapter - and, more to the point,
	// without leaving one adapter that forgot to thread it and quietly shares a pod between
	// two instances that were supposed to be separate.
	//
	// An un-instanced app is unchanged: Runtime() gives back the bare name.
	cfg.AppNameID = addr.Runtime()
	if err := expandMounts(&cfg, addr); err != nil {
		return schema.AppConfig{}, err
	}
	return cfg, nil
}

// expandMounts resolves {state}/{app}/{instance} in the app's host mount paths, so one
// definition can serve many instances without each one needing its own copy of the config
// just to point at its own directory.
//
// Only the HOST side is templated. The container side is the path the app looks at, which is
// the same for every instance by design - the whole point is that the app is unaware there is
// more than one of it, and an app told to find its profile at a different path per instance
// would have to be configured per instance too.
func expandMounts(cfg *schema.AppConfig, addr paths.Address) error {
	for index := range cfg.Volumes {
		expanded, err := addr.Expand(cfg.Volumes[index].HostMount)
		if err != nil {
			return fmt.Errorf("Volumes[%d]: %w", index, err)
		}
		cfg.Volumes[index].HostMount = expanded
	}
	for index := range cfg.Configs {
		expanded, err := addr.Expand(cfg.Configs[index].HostMount)
		if err != nil {
			return fmt.Errorf("Configs[%d]: %w", index, err)
		}
		cfg.Configs[index].HostMount = expanded
	}
	return nil
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

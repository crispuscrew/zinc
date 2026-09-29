package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/creator/internal/advisory"
	"github.com/crispuscrew/zinc/creator/internal/backend"
)

const newUsage = "usage: zc new <name> --image <image> [--vm --base-digest sha256:...] [options; --help lists them]"

func cmdNew(svc backend.Service, argv []string) error {
	if len(argv) == 0 || strings.HasPrefix(argv[0], "-") {
		return fmt.Errorf("%s", newUsage)
	}
	cfg := schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer, AppNameID: argv[0]}
	opts := vmoptions.Default(cfg.AppNameID, "")
	fset := flag.NewFlagSet("new", flag.ContinueOnError)
	isVM := fset.Bool("vm", false, "author a VM app and its runtime options together")
	registerShared(fset, &cfg)
	vmFlags := registerVM(fset, &cfg, &opts)
	if err := fset.Parse(argv[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fset.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q (flags must follow the name)", fset.Arg(0))
	}
	if *isVM {
		cfg.Type = schema.ZincVirtualization
		if err := vmFlags.apply(fset, &cfg, &opts); err != nil {
			return err
		}
	} else if vmFlagUsed(fset) {
		return fmt.Errorf("VM flags were given without --vm; add --vm to author a VM app")
	} else {
		cfg.ImageMeta.CloudInit = false
	}
	if cfg.Inherits != "" {
		return fmt.Errorf("new --inherits requires presence-aware file editing; author the child YAML directly so omitted values remain inherited")
	}
	keepIDImplied := !cfg.DBusMeta.IsZero() && !cfg.InternalUserMeta.KeepUserID
	if !cfg.DBusMeta.IsZero() {
		cfg.InternalUserMeta.KeepUserID = true
	}
	var settings *vmoptions.Config
	if *isVM {
		opts.AppNameID, opts.Image = cfg.AppNameID, cfg.ImageMeta.Image
		settings = &opts
	}
	if err := svc.Create(cfg, settings); err != nil {
		return err
	}
	fmt.Printf("created %s -> %s\n", cfg.AppNameID, svc.Path(cfg.AppNameID))
	if keepIDImplied {
		fmt.Println("note: set InternalUserMeta.KeepUserID for the filtered session bus")
	}
	if settings != nil {
		fmt.Printf("VM runtime options -> %s\n", svc.VMPath(cfg.AppNameID))
	}
	for _, warning := range advisory.Warnings(cfg) {
		fmt.Println("warning: " + warning)
	}
	return nil
}

func registerShared(flags *flag.FlagSet, cfg *schema.AppConfig) {
	flags.StringVar(&cfg.ImageMeta.Image, "image", "", "digest-pinned container reference or VM base disk path")
	flags.StringVar(&cfg.LauncherMeta.Description, "desc", "", "launcher description")
	flags.StringVar(&cfg.LauncherMeta.Icon, "icon", "", "launcher icon name or path")
	flags.StringVar(&cfg.LauncherMeta.Group, "group", "", "launcher group")
	flags.StringVar(&cfg.Inherits, "inherits", "", "parent app (requires editing a sparse YAML file)")
	flags.StringVar(&cfg.StartConditions.Entrypoint, "entrypoint", "", "primary command; empty uses the image default")
	flags.StringVar(&cfg.StartConditions.AttachedEntrypoint, "attached-entrypoint", "", "attached command; empty uses Entrypoint")
	flags.BoolVar(&cfg.StartConditions.Terminal, "terminal", false, "open a terminal")
	flags.BoolVar(&cfg.StartConditions.Attached, "attached", false, "attached sessions hold the app alive")
	flags.BoolVar(&cfg.StartConditions.ReadOnlyRootfs, "read-only-rootfs", false, "read-only root filesystem")
	flags.BoolVar(&cfg.StopConditions.Autorestart, "autorestart", false, "restart after failure, not manual stop")
	flags.BoolVar(&cfg.StopConditions.KeepAlive, "keep-alive", false, "keep the app after its entrypoint finishes")
	flags.BoolVar(&cfg.StopConditions.Background, "background", false, "keep running after the window closes")
	flags.BoolVar(&cfg.MinimizeFingerprint, "minimize-fingerprint", false, "best-effort reduction of guest-visible fingerprints")
	flags.Float64Var(&cfg.ResourcesMeta.MaxCPUCores, "cpus", 0, "CPU limit; VM requires whole cores")
	flags.Int64Var(&cfg.ResourcesMeta.MaxRamMiB, "ram", 0, "RAM in MiB")
	flags.Int64Var(&cfg.ResourcesMeta.PIDsLimit, "pids", 0, "container process limit")
	flags.StringVar(&cfg.InternalUserMeta.NonRootUserName, "user", "", "non-root/guest user name")
	flags.BoolVar(&cfg.InternalUserMeta.UseNonRootUser, "non-root", false, "use the named non-root user")
	flags.BoolVar(&cfg.InternalUserMeta.KeepUserID, "keep-user-id", false, "preserve host user identity")
	flags.BoolVar(&cfg.HostTheme, "host-theme", false, "share the host theme")
	flags.StringVar(&cfg.ImageMeta.SourceTag, "source-tag", "", "image pin provenance")
	flags.Func("install", "semicolon-separated setup steps", func(value string) error {
		for _, step := range strings.Split(value, ";") {
			if step = strings.TrimSpace(step); step != "" {
				cfg.ImageMeta.Install = append(cfg.ImageMeta.Install, step)
			}
		}
		return nil
	})
	flags.Func("depends-on", "dependency names, comma-separated", func(value string) error {
		cfg.StartConditions.DependsOn = append(cfg.StartConditions.DependsOn, splitList(value)...)
		return nil
	})
	flags.Func("env", "primary NAME=VALUE; repeatable, no shell expansion", envFlag(&cfg.StartConditions.EntrypointEnv))
	flags.Func("attached-env", "attached NAME=VALUE; repeatable, no shell expansion", envFlag(&cfg.StartConditions.AttachedEnv))
	flags.Func("creator-flag", "one raw build argument; repeat to preserve argv boundaries", appendArg(&cfg.CreatorFlags))
	flags.Func("runner-flag", "one raw runtime argument; repeat to preserve argv boundaries", appendArg(&cfg.RunnerFlags))
	flags.Func("creator-flags", "raw build argv as a YAML/JSON string array", decodeFlag(&cfg.CreatorFlags))
	flags.Func("runner-flags", "raw runtime argv as a YAML/JSON string array", decodeFlag(&cfg.RunnerFlags))
	flags.Func("dbus-talk", "comma-separated bus names", func(value string) error { cfg.DBusMeta.Talk = splitList(value); return nil })
	flags.Func("dbus-own", "comma-separated owned bus names", func(value string) error { cfg.DBusMeta.Own = splitList(value); return nil })
	flags.BoolVar(&cfg.DisplayMeta.DisableGpuAccess, "disable-gpu", false, "disable GPU access")
	flags.BoolVar(&cfg.DisplayMeta.DisableSecurityContext, "disable-security-context", false, "use the unfiltered display socket")
	flags.BoolVar(&cfg.DisplayMeta.RequireSecurityContext, "require-security-context", false, "require a display security context")
	registerComplex(flags, cfg)
	flags.Func("tunnel", "removed; author networking explicitly", func(string) error {
		return fmt.Errorf("--tunnel is not representable in schema v4; no tunnel or network grants were created")
	})
	flags.Func("capability", "removed; raw RunnerFlags require explicit review", func(string) error {
		return fmt.Errorf("--capability is not representable in schema v4; use explicit raw RunnerFlags with their warning")
	})
	flags.Func("ready-check", "removed readiness setting", func(string) error {
		return fmt.Errorf("--ready-check is not representable in schema v4; dependency readiness cannot be inferred")
	})
}

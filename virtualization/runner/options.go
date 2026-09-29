package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	store "github.com/crispuscrew/zinc/common/adapters/vmoptions"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/app"
)

type mediaList []string

func (list *mediaList) String() string { return strings.Join(*list, ",") }
func (list *mediaList) Set(value string) error {
	*list = append(*list, value)
	return nil
}

func launchConfig(svc app.Service, argv []string) (schema.AppConfig, vmoptions.Config, bool, error) {
	var cfg schema.AppConfig
	var runtime vmoptions.Config
	if len(argv) == 0 || strings.HasPrefix(argv[0], "-") {
		return cfg, runtime, false, fmt.Errorf("an app name or path must precede runtime options")
	}
	fset := flag.NewFlagSet("VM runtime", flag.ContinueOnError)
	file := fset.String("runtime-options", "", "explicit runtime options JSON")
	dry := fset.Bool("dry-run", false, "print without changing anything")
	pin := fset.String("base-digest", "", "authorized base digest")
	size := fset.Int64("disk", 0, "initial overlay size GiB")
	display := fset.String("display", "", "display profile")
	devices := fset.String("devices", "", "device profile")
	clearMedia := fset.Bool("clear-media", false, "clear stored media")
	clearForwards := fset.Bool("clear-forwards", false, "clear stored forwards")
	var media, forwards, raw mediaList
	fset.Var(&media, "media", "read-only ISO; repeatable")
	fset.Var(&forwards, "forward", "HOST:GUEST[/TCP|UDP][@NIC]; repeatable")
	fset.Var(&raw, "runner-arg", "raw QEMU argv element; repeatable")
	if err := fset.Parse(argv[1:]); err != nil {
		return cfg, runtime, false, err
	}
	if fset.NArg() != 0 {
		return cfg, runtime, false, fmt.Errorf("unexpected positional argument %q", fset.Arg(0))
	}
	var err error
	cfg, err = loadApp(svc, argv[0])
	if err != nil {
		return cfg, runtime, false, err
	}
	if cfg.Type != schema.ZincVirtualization {
		return cfg, runtime, false, fmt.Errorf("%s is not a VM; use zcr", cfg.AppNameID)
	}
	runtime = vmoptions.Default(cfg.AppNameID, cfg.ImageMeta.Image)
	if *file != "" {
		runtime, err = store.LoadFile(*file)
	} else if strings.Contains(argv[0], "/") || strings.HasSuffix(argv[0], ".yaml") {
		if *pin == "" {
			err = fmt.Errorf("an app loaded by path needs --runtime-options or an explicit --base-digest")
		}
	} else {
		loaded, loadErr := store.Load(filepath.Dir(filepath.Dir(svc.Store.Root)), cfg.AppNameID)
		if loadErr == nil {
			runtime = loaded
		} else if !os.IsNotExist(loadErr) {
			err = loadErr
		}
	}
	if err != nil {
		return cfg, runtime, false, err
	}
	fset.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "base-digest":
			runtime.BaseDigest = *pin
		case "disk":
			runtime.DiskSizeGiB = *size
		case "display":
			runtime.Display = vmoptions.Display(*display)
		case "devices":
			runtime.Devices = vmoptions.Devices(*devices)
		}
	})
	if *clearMedia || len(media) > 0 {
		runtime.InstallMedia = append([]string(nil), media...)
	}
	if *clearForwards || len(forwards) > 0 {
		runtime.ForwardPorts = nil
		for _, spec := range forwards {
			forward, parseErr := parseForward(spec)
			if parseErr != nil {
				return cfg, runtime, false, parseErr
			}
			runtime.ForwardPorts = append(runtime.ForwardPorts, forward)
		}
	}
	cfg.RunnerFlags = append(cfg.RunnerFlags, raw...)
	return cfg, runtime, *dry, vmoptions.Validate(runtime)
}

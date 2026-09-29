package main

import (
	"fmt"
	"sort"

	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/adapters/podman"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func cmdBuild(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr build <app>")
	}
	cfg, err := loadLaunchable(svc, argv[0])
	if err != nil {
		return err
	}
	if !derived.HasInstall(cfg) {
		return fmt.Errorf("%s: no ImageMeta.Install or CreatorFlags set - nothing to build; it runs %s directly", cfg.AppNameID, cfg.ImageMeta.Image)
	}
	for _, warning := range validate.Warnings(cfg) {
		fmt.Println("WARNING: " + warning)
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
	cfg, err := loadLaunchable(svc, argv[0])
	if err != nil {
		return err
	}
	fmt.Printf("ok: %s - image=%s\n", cfg.AppNameID, cfg.ImageMeta.Image)
	for _, warning := range validate.Warnings(cfg) {
		fmt.Println("warning: " + warning)
	}
	return nil
}

func cmdLifecycle(svc app.Service, opt options.HostOptions, command string, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr %s <app>", command)
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	switch command {
	case "inspect":
		return svc.Do(podman.InspectArgs(cfg.AppNameID))
	case "stop":
		return svc.Stop(cfg)
	case "restart":
		if len(cfg.NetworkMeta.RulesByPriority) > 0 || len(cfg.NetworkMeta.Interfaces) > 0 {
			if err := svc.Stop(cfg); err != nil {
				return err
			}
			return svc.Launch(cfg, opt)
		}
		return svc.Do(podman.RestartArgs(cfg.AppNameID))
	}
	return fmt.Errorf("unreachable: %q", command)
}

func cmdLogs(svc app.Service, argv []string) error {
	name, follow := "", false
	for _, arg := range argv {
		if arg == "-f" {
			follow = true
		} else if name == "" {
			name = arg
		} else {
			return fmt.Errorf("usage: zcr logs <app> [-f]")
		}
	}
	if name == "" {
		return fmt.Errorf("usage: zcr logs <app> [-f]")
	}
	cfg, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	return svc.Do(podman.LogsArgs(cfg.AppNameID, follow))
}

func cmdPs(svc app.Service) error {
	running, err := svc.Running()
	if err != nil {
		return err
	}
	var names []string
	for name, active := range running {
		if active {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Println(name)
	}
	return nil
}

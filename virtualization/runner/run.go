package main

import (
	"fmt"
	"os"

	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/virtualization/runner/app"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

func cmdRun(svc app.Service, argv []string) error {
	cfg, runtime, dryRun, err := launchConfig(svc, argv)
	if err != nil {
		return err
	}
	svc.Options = runtime
	if dryRun {
		args, ruleset, err := svc.Plan(cfg)
		if err != nil {
			return err
		}
		for _, warning := range qemu.Warnings(cfg, runtime, false) {
			fmt.Fprintln(os.Stderr, "warning: "+warning)
		}
		if ruleset != "" {
			fmt.Printf("# namespace rules loaded before QEMU starts:\n%s\n", ruleset)
		}
		if len(cfg.StartConditions.DependsOn) > 0 {
			fmt.Printf("# dependencies checked and started first: %v\n", cfg.StartConditions.DependsOn)
		}
		if len(cfg.RunnerFlags) > 0 {
			fmt.Printf("# %d raw RunnerFlags omitted from plan (values may contain secrets)\n", len(cfg.RunnerFlags))
		}
		fmt.Println(qemu.Display(args))
		return nil
	}
	if err := svc.Run(cfg); err != nil {
		return err
	}
	fmt.Printf("started %s (%d MiB, %g vCPU, display %s)\n", cfg.AppNameID,
		cfg.ResourcesMeta.MaxRamMiB, cfg.ResourcesMeta.MaxCPUCores, qemu.ResolveDisplay(cfg, runtime))
	return nil
}

func cmdValidate(svc app.Service, argv []string) error {
	cfg, runtime, _, err := launchConfig(svc, argv)
	if err != nil {
		return err
	}
	svc.Options = runtime
	if err := svc.Validate(cfg); err != nil {
		return err
	}
	fmt.Printf("%s: app and runtime options valid (host capabilities and disk contents not verified)\n", cfg.AppNameID)
	for _, warning := range append(validate.Warnings(cfg), qemu.Warnings(cfg, runtime, false)...) {
		fmt.Fprintln(os.Stderr, "warning: "+warning)
	}
	return nil
}

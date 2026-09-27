package main

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/creator/internal/advisory"
	"github.com/crispuscrew/zinc/creator/internal/backend"
)

func cmdList(svc backend.Service) error {
	names, err := svc.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("no apps defined yet - create one with: zc new <name> --image <img>")
	}
	for _, name := range names {
		cfg, err := svc.LoadResolved(name)
		if err != nil {
			fmt.Printf("%-20s (error: %v)\n", name, err)
			continue
		}
		fmt.Printf("%-20s %-4s %-12s %s\n", name, kindLabel(cfg), traitLabel(cfg), cfg.ImageMeta.Image)
	}
	return nil
}

func cmdValidate(svc backend.Service, argv []string) error {
	if len(argv) < 1 || len(argv) > 2 {
		return fmt.Errorf("usage: zc validate <name|app.yaml> [--resolved]")
	}
	resolved := len(argv) == 2 && argv[1] == "--resolved"
	if len(argv) == 2 && !resolved {
		return fmt.Errorf("unknown flag %q", argv[1])
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	if resolved {
		data, err := svc.Marshal(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("# %s as it resolves\n%s\n", cfg.AppNameID, data)
	}
	if err := validate.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config %s:\n%w", argv[0], err)
	}
	if cfg.Type == schema.ZincVirtualization {
		if _, err := svc.LoadVM(cfg); err != nil {
			return err
		}
	}
	fmt.Printf("ok: %s - %s image=%s %s\n", cfg.AppNameID, kindLabel(cfg), cfg.ImageMeta.Image, traitLabel(cfg))
	if cfg.Inherits != "" && !resolved {
		fmt.Printf("inherits from %s; use --resolved to inspect\n", cfg.Inherits)
	}
	for _, warn := range advisory.Warnings(cfg) {
		fmt.Println("warning: " + warn)
	}
	return nil
}

func cmdDelete(svc backend.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zc delete <name>")
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

func loadApp(svc backend.Service, arg string) (schema.AppConfig, error) {
	if strings.Contains(arg, "/") || strings.HasSuffix(arg, ".yaml") {
		return svc.LoadFileResolved(arg)
	}
	return svc.LoadResolved(arg)
}

func netLabel(cfg schema.AppConfig) string {
	if len(cfg.NetworkMeta.Interfaces) == 0 {
		return "no NIC"
	}
	return fmt.Sprintf("net:%d/%d", len(cfg.NetworkMeta.Interfaces), len(cfg.NetworkMeta.RulesByPriority))
}

func kindLabel(cfg schema.AppConfig) string {
	if cfg.Type == schema.ZincVirtualization {
		return "vm"
	}
	return "ctr"
}

func traitLabel(cfg schema.AppConfig) string {
	if cfg.Type == schema.ZincVirtualization {
		return fmt.Sprintf("%dM/%gcpu", cfg.ResourcesMeta.MaxRamMiB, cfg.ResourcesMeta.MaxCPUCores)
	}
	return netLabel(cfg)
}

package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
)

func loadApp(svc app.Service, arg string) (schema.AppConfig, error) {
	cfg, err := load(svc, arg)
	if err != nil {
		return schema.AppConfig{}, err
	}
	if cfg.Type == schema.ZincVirtualization {
		return schema.AppConfig{}, fmt.Errorf("app %q is a VM app (Type: %s); run it with zvr", cfg.AppNameID, cfg.Type)
	}
	if err := validate.AppName(cfg.AppNameID); err != nil {
		return schema.AppConfig{}, fmt.Errorf("%s: %w", arg, err)
	}
	return cfg, nil
}

func loadLaunchable(svc app.Service, arg string) (schema.AppConfig, error) {
	cfg, err := loadApp(svc, arg)
	if err != nil {
		return schema.AppConfig{}, err
	}
	if err := validate.Validate(cfg); err != nil {
		return schema.AppConfig{}, err
	}
	return cfg, nil
}

func refuseForgedIdentity(svc app.Service, arg string, cfg schema.AppConfig) error {
	claimed := strings.TrimSpace(cfg.AppNameID)
	if claimed == "" || !svc.Exists(claimed) {
		return nil
	}
	stored, storeErr := filepath.Abs(svc.Path(claimed))
	given, givenErr := filepath.Abs(arg)
	if storeErr == nil && givenErr == nil && stored == given {
		return nil
	}
	return fmt.Errorf("%s claims AppNameID %q, which is a different app in the store; rename it or run the store's own app by name", arg, claimed)
}

func refuseVM(svc app.Service, name string) error {
	cfg, err := load(svc, name)
	if err != nil || cfg.Type != schema.ZincVirtualization {
		return nil
	}
	return fmt.Errorf("app %q is a VM app (Type: %s); use zvr", name, cfg.Type)
}

func load(svc app.Service, arg string) (schema.AppConfig, error) {
	if strings.Contains(arg, "/") || strings.HasSuffix(arg, ".yaml") {
		cfg, err := svc.LoadFileResolved(arg)
		if err != nil {
			return schema.AppConfig{}, err
		}
		if err := refuseForgedIdentity(svc, arg, cfg); err != nil {
			return schema.AppConfig{}, err
		}
		return cfg, nil
	}
	address, err := paths.ParseAddress(arg)
	if err != nil {
		return schema.AppConfig{}, err
	}
	if !svc.Exists(address.App) {
		return schema.AppConfig{}, fmt.Errorf("no app %q defined (try: zc list)", address.App)
	}
	cfg, err := svc.LoadResolved(address.App)
	if err != nil {
		return schema.AppConfig{}, err
	}
	cfg.AppNameID = address.Runtime()
	if err := expandMounts(&cfg, address); err != nil {
		return schema.AppConfig{}, err
	}
	return cfg, nil
}

func expandMounts(cfg *schema.AppConfig, address paths.Address) error {
	for index := range cfg.Volumes {
		expanded, err := address.Expand(cfg.Volumes[index].HostMount)
		if err != nil {
			return fmt.Errorf("Volumes[%d]: %w", index, err)
		}
		cfg.Volumes[index].HostMount = expanded
	}
	return nil
}

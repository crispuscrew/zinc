package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

func (svc Service) teardownSteps(cfg schema.AppConfig) []ports.Command {
	return append(svc.bus.Teardown(cfg), svc.net.Teardown(cfg)...)
}

func (svc Service) Stop(cfg schema.AppConfig) error { return svc.runAll(svc.teardownSteps(cfg)) }

func (svc Service) teardown(cfg schema.AppConfig, hadSteps bool) error {
	if !hadSteps {
		return nil
	}
	return svc.runAll(svc.teardownSteps(cfg))
}

func (svc Service) runAll(steps []ports.Command) error {
	var failures []error
	for _, step := range steps {
		if err := svc.runtime.Exec(step); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (svc Service) NetCounters(cfg schema.AppConfig, opt options.HostOptions) (string, bool, error) {
	command, filtered := svc.net.Counters(cfg, opt)
	if !filtered {
		return "", false, nil
	}
	output, err := svc.runtime.Capture(command)
	return output, true, err
}

func (svc Service) Rename(oldName, newName string) error {
	oldName, newName = strings.TrimSpace(oldName), strings.TrimSpace(newName)
	switch {
	case newName == "":
		return fmt.Errorf("rename %s: new name must not be empty", oldName)
	case newName == oldName:
		return fmt.Errorf("rename %s: new name is unchanged", oldName)
	case svc.store.Exists(newName):
		return fmt.Errorf("rename %s: %q already exists", oldName, newName)
	}
	running, err := svc.runtime.Running()
	if err != nil {
		return err
	}
	if running[oldName] {
		return fmt.Errorf("rename %s: app is running - stop it first (its container is named %q)", oldName, oldName)
	}
	cfg, err := svc.store.Load(oldName)
	if err != nil {
		return fmt.Errorf("rename %s: %w", oldName, err)
	}
	cfg.AppNameID = newName
	if err := svc.store.Save(cfg); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", oldName, newName, err)
	}
	if err := svc.store.Delete(oldName); err != nil {
		return fmt.Errorf("rename %s -> %s: saved new definition but could not remove the old one: %w", oldName, newName, err)
	}
	return nil
}

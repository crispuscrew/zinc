package app

import (
	"errors"
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

func (svc Service) ensureHolder(cfg schema.AppConfig, opt options.HostOptions) error {
	if svc.runtime.IsRunning(cfg.AppNameID) {
		return nil
	}
	opt = svc.withBundle(cfg, opt)
	if err := checkLaunchSources(cfg, opt); err != nil {
		return err
	}
	// A stopped KeepAlive/Autorestart holder and its pod must be removed before reuse.
	if svc.runtime.Exists(cfg.AppNameID) {
		if err := svc.Stop(cfg); err != nil {
			return err
		}
	}
	steps, err := svc.prepareSteps(cfg, opt)
	if err != nil {
		return err
	}
	fail := func(err error) error { return errors.Join(err, svc.teardown(cfg, len(steps) > 0)) }
	for _, command := range steps {
		if err := svc.runtime.Exec(command); err != nil {
			return fail(fmt.Errorf("start %s (%s): %w", cfg.AppNameID, command.Desc, err))
		}
	}
	opt, err = svc.establish(cfg, opt)
	if err != nil {
		return fail(fmt.Errorf("start %s: %w", cfg.AppNameID, err))
	}
	args, err := svc.runtime.AppRunArgs(cfg, opt, svc.attachFlags(cfg, opt))
	if err != nil {
		return fail(err)
	}
	if err := svc.runtime.Exec(ports.Command{Args: args, Desc: "start holder"}); err != nil {
		return fail(fmt.Errorf("start %s holder: %w", cfg.AppNameID, err))
	}
	return nil
}

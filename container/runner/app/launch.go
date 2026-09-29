package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/adapters/ipc"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

func (svc Service) Plan(cfg schema.AppConfig, opt options.HostOptions) ([]ports.Command, error) {
	if err := validate.Validate(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", cfg.AppNameID, err)
	}
	opt = svc.withBundle(cfg, opt)
	steps, err := svc.prepareSteps(cfg, opt)
	if err != nil {
		return nil, err
	}
	// This placeholder never names the session socket and is not established by a dry run.
	if wantsPipeWire(cfg.AudioMeta) {
		opt.PipeWireSocket = "/run/zinc-planned/" + cfg.AppNameID + "/pipewire-0"
	}
	args, err := svc.runtime.AppRunArgs(cfg, opt, svc.attachFlags(cfg, opt))
	if err != nil {
		return nil, err
	}
	description := "run " + cfg.AppNameID
	if cfg.StartConditions.Attached {
		description = "run holder for " + cfg.AppNameID + " (terminals exec in)"
	}
	return append(steps, ports.Command{Args: args, Desc: description}), nil
}

func (svc Service) Launch(cfg schema.AppConfig, opt options.HostOptions) error {
	return svc.launch(cfg, opt, nil, map[string]bool{})
}

func launchWarnings(cfg schema.AppConfig) {
	for _, warning := range validate.Warnings(cfg) {
		fmt.Fprintln(os.Stderr, "WARNING: "+warning)
	}
}

func (svc Service) launch(cfg schema.AppConfig, opt options.HostOptions, chain []string, started map[string]bool) error {
	if started[cfg.AppNameID] {
		return nil
	}
	if err := validate.Validate(cfg); err != nil {
		return fmt.Errorf("%s: %w", cfg.AppNameID, err)
	}
	if !cfg.StartConditions.Attached {
		launchWarnings(cfg)
	}
	lock := lockLaunch(cfg.AppNameID)
	defer lock.close()
	running, err := svc.runtime.Running()
	if err != nil {
		return err
	}
	if running[cfg.AppNameID] && cfg.StartConditions.Attached {
		return svc.OpenTerminal(cfg, opt, false)
	}
	if running[cfg.AppNameID] && !cfg.StartConditions.Attached {
		return fmt.Errorf("%s is already running; stop it first, or run another instance with %s@<instance>", cfg.AppNameID, cfg.AppNameID)
	}
	opt = svc.withBundle(cfg, opt)
	if err := checkLaunchSources(cfg, opt); err != nil {
		return err
	}
	steps, err := svc.prepareSteps(cfg, opt)
	if err != nil {
		return err
	}
	started[cfg.AppNameID] = true
	if err := svc.startDependencies(cfg, opt, chain, started); err != nil {
		return err
	}
	if cfg.StartConditions.Attached {
		return svc.OpenTerminal(cfg, opt, false)
	}
	if err := svc.ensureImage(cfg); err != nil {
		return err
	}
	fail := func(err error) error { return errors.Join(err, svc.teardown(cfg, len(steps) > 0)) }
	for _, command := range steps {
		if err := svc.runtime.Exec(command); err != nil {
			return fail(fmt.Errorf("launch %s (%s): %w", cfg.AppNameID, command.Desc, err))
		}
	}
	opt, err = svc.establish(cfg, opt)
	if err != nil {
		return fail(fmt.Errorf("launch %s: %w", cfg.AppNameID, err))
	}
	args, err := svc.runtime.AppRunArgs(cfg, opt, svc.attachFlags(cfg, opt))
	if err != nil {
		return fail(err)
	}
	var supervisor *ipc.Child
	if len(steps) > 0 {
		supervisor, err = startSupervisor(cfg)
		if err != nil {
			return fail(err)
		}
	}
	onFail := func() { _ = svc.teardown(cfg, len(steps) > 0) }
	if err := svc.runtime.StartApp(cfg, opt, args, onFail); err != nil {
		if supervisor != nil {
			err = errors.Join(err, supervisor.Close())
		}
		return fail(err)
	}
	if supervisor != nil {
		if err := supervisor.Commit(); err != nil {
			return fail(errors.Join(fmt.Errorf("activate supervisor: %w", err), supervisor.Close()))
		}
	}
	return nil
}

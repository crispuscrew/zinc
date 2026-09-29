package app

import (
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/session"
)

const defaultShell = "/bin/sh"

// TerminalRequest carries the resolved definition and per-launch options across re-exec.
// Reloading by container name loses instances, file launches and runtime-only volumes.
type TerminalRequest struct {
	Config  schema.AppConfig
	Options options.HostOptions
}

func (svc Service) validateTerminal(cfg schema.AppConfig, opt options.HostOptions) error {
	if err := validate.Validate(cfg); err != nil {
		return fmt.Errorf("%s: %w", cfg.AppNameID, err)
	}
	if !cfg.StartConditions.Attached {
		return fmt.Errorf("%s: not an attached app", cfg.AppNameID)
	}
	if len(opt.Terminal) == 0 {
		return fmt.Errorf("%s: terminal app but no terminal emulator configured (set ZINC_TERMINAL)", cfg.AppNameID)
	}
	return nil
}

func (svc Service) OpenTerminal(cfg schema.AppConfig, opt options.HostOptions, shell bool) error {
	if err := svc.validateTerminal(cfg, opt); err != nil {
		return err
	}
	launchWarnings(cfg)
	opt = svc.withBundle(cfg, opt)
	if !svc.runtime.IsRunning(cfg.AppNameID) {
		if err := checkLaunchSources(cfg, opt); err != nil {
			return err
		}
		if _, err := svc.prepareSteps(cfg, opt); err != nil {
			return err
		}
		if err := svc.ensureImage(cfg); err != nil {
			return err
		}
	}
	return spawnTerminal(cfg, opt, shell)
}

func (svc Service) Term(cfg schema.AppConfig, opt options.HostOptions, shell bool) error {
	if err := svc.validateTerminal(cfg, opt); err != nil {
		return err
	}
	root, err := runRoot()
	if err != nil {
		return err
	}
	wait := &waiter{
		runRoot: root, background: cfg.StopConditions.Background,
		ensureUp: func() error {
			err := svc.ensureHolder(cfg, opt)
			if err != nil {
				reportTerm("error " + err.Error())
			} else {
				reportTerm("ok")
			}
			return err
		},
		runTerminal: func() error { return svc.runTerminalSession(cfg, opt, shell) },
		stop:        func() error { return svc.Stop(cfg) },
	}
	return wait.run(cfg.AppNameID)
}

func (svc Service) runTerminalSession(cfg schema.AppConfig, opt options.HostOptions, shell bool) error {
	command := multitermCmd(cfg)
	if shell {
		command = []string{defaultShell}
	}
	return svc.runtime.OpenSession(cfg.AppNameID, command, session.Environment(cfg.StartConditions), opt, false)
}

func multitermCmd(cfg schema.AppConfig) []string { return session.Command(cfg.StartConditions) }

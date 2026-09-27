package app

import (
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

func (svc Service) withBundle(cfg schema.AppConfig, opt options.HostOptions) options.HostOptions {
	opt.BundleDir = paths.BundleDir(opt.ConfigHome, svc.address(cfg.AppNameID).App)
	return opt
}

func (svc Service) withDisplay(cfg schema.AppConfig, opt options.HostOptions) (options.HostOptions, error) {
	if svc.display == nil {
		return opt, nil
	}
	socket, err := svc.display.Establish(svc.address(cfg.AppNameID), cfg, opt)
	opt.WaylandSocket = socket
	return opt, err
}

func (svc Service) withAudio(cfg schema.AppConfig, opt options.HostOptions) (options.HostOptions, error) {
	if svc.audio == nil {
		return opt, nil
	}
	socket, err := svc.audio.Establish(svc.address(cfg.AppNameID), cfg, opt)
	opt.PipeWireSocket = socket
	return opt, err
}

func (svc Service) withNotify(cfg schema.AppConfig, opt options.HostOptions) (options.HostOptions, error) {
	if svc.notify == nil {
		return opt, nil
	}
	socket, err := svc.notify.Establish(svc.address(cfg.AppNameID), cfg, opt)
	opt.NotifySocket = socket
	return opt, err
}

func (svc Service) attachFlags(cfg schema.AppConfig, opt options.HostOptions) []string {
	flags := svc.net.RunFlags(cfg)
	if opt.NotifySocket == "" {
		flags = append(flags, svc.bus.RunFlags(cfg)...)
	}
	return flags
}

func (svc Service) prepareSteps(cfg schema.AppConfig, opt options.HostOptions) ([]ports.Command, error) {
	steps, err := svc.net.Prepare(cfg, opt)
	if err != nil {
		return nil, err
	}
	if cfg.MinimizeFingerprint {
		// The enforcer owns the shared UTS namespace. Decorate its pod creation,
		// never the joining app or the transient firewall helper.
		for index := range steps {
			args := steps[index].Args
			if len(args) >= 2 && args[0] == "pod" && args[1] == "create" && !slices.ContainsFunc(args, func(arg string) bool {
				return arg == "--hostname" || strings.HasPrefix(arg, "--hostname=")
			}) {
				steps[index].Args = append(slices.Clone(args), "--hostname", "localhost")
			}
		}
	}
	busSteps, err := svc.bus.Prepare(cfg)
	if err != nil {
		return nil, err
	}
	return append(steps, busSteps...), nil
}

func (svc Service) establish(cfg schema.AppConfig, opt options.HostOptions) (options.HostOptions, error) {
	var err error
	for _, establish := range []func(schema.AppConfig, options.HostOptions) (options.HostOptions, error){svc.withDisplay, svc.withAudio, svc.withNotify} {
		opt, err = establish(cfg, opt)
		if err != nil {
			return opt, err
		}
	}
	return opt, nil
}

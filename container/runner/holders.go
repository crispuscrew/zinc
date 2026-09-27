package main

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/container/runner/adapters/dbusproxy"
	"github.com/crispuscrew/zinc/container/runner/adapters/notifyfilter"
	"github.com/crispuscrew/zinc/container/runner/adapters/pipewirectx"
	"github.com/crispuscrew/zinc/container/runner/adapters/podman"
	"github.com/crispuscrew/zinc/container/runner/adapters/waylandctx"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
)

func cmdWaylandHolder(opt options.HostOptions, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr %s <app[@instance]>", waylandctx.HoldCommand)
	}
	address, err := paths.ParseAddress(argv[0])
	if err != nil {
		return err
	}
	if strings.TrimSpace(address.App) == "" {
		return fmt.Errorf("usage: zcr %s <app[@instance]>", waylandctx.HoldCommand)
	}
	return waylandctx.Hold(address, opt, podman.WaitGone)
}

func cmdSupervise(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr __supervise <runtime-name> (inherited launch snapshot required)")
	}
	return svc.HoldSupervisor(argv[0], podman.WaitGone)
}

func cmdNotifyFilter(svc app.Service, opt options.HostOptions, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr %s <app[@instance]>", notifyfilter.HoldCommand)
	}
	cfg, err := load(svc, argv[0])
	if err != nil {
		return err
	}
	upstream := dbusproxy.HostSocketPath(opt.RuntimeDir, cfg.AppNameID)
	if upstream == "" {
		return fmt.Errorf("%s: no bus socket to filter", cfg.AppNameID)
	}
	return notifyfilter.Hold(cfg, upstream, opt, podman.WaitGone)
}

func cmdPipeWireHolder(opt options.HostOptions, argv []string) error {
	usage := fmt.Errorf("usage: zcr %s <app[@instance]> [%s]", pipewirectx.HoldCommand, pipewirectx.MicrophoneFlag)
	if len(argv) < 1 || len(argv) > 2 {
		return usage
	}
	address, err := paths.ParseAddress(argv[0])
	if err != nil {
		return err
	}
	if strings.TrimSpace(address.App) == "" {
		return usage
	}
	capture := false
	if len(argv) == 2 {
		if argv[1] != pipewirectx.MicrophoneFlag {
			return usage
		}
		capture = true
	}
	return pipewirectx.Hold(address, capture, opt, podman.WaitGone)
}

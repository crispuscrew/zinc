package app

import (
	"fmt"
	"reflect"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/firmware"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/netns"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

type launchPlan struct {
	Authored      schema.AppConfig
	RuntimeConfig schema.AppConfig
	Layout        qemu.Layout
	Network       *provision.Manifest
	Lookup        provision.Lookup
}

// Resolution never changes authored intent. A manifest-assigned MAC belongs only
// to this launch, not the app file or the supervisor's future restart requests.
func (svc Service) resolvePlan(cfg schema.AppConfig) (launchPlan, error) {
	plan := launchPlan{Authored: cfg, RuntimeConfig: cfg, Layout: svc.layout(cfg), Lookup: svc.Lookup}
	if err := svc.check(cfg); err != nil {
		return plan, err
	}
	if netns.Applies(cfg) {
		load := svc.LoadNetwork
		if load == nil {
			load = provision.Load
		}
		manifest, err := load(cfg)
		if err != nil {
			return plan, err
		}
		if err := provision.CheckForwards(manifest, svc.Options.ForwardPorts); err != nil {
			return plan, err
		}
		bound, attachments, err := netns.ConfigureResolved(cfg, manifest)
		if err != nil {
			return plan, err
		}
		if len(svc.NetworkAttachments) > 0 && !reflect.DeepEqual(svc.NetworkAttachments, attachments) {
			return plan, fmt.Errorf("manual network attachments differ from provisioned topology")
		}
		plan.RuntimeConfig, plan.Network = bound, &manifest
		plan.Layout.NetworkAttachments, plan.Layout.Namespaced = attachments, true
	}
	audio, err := qemu.PlanAudio(cfg.AudioMeta)
	if err != nil {
		return plan, err
	}
	plan.Layout.Audio = audio
	return plan, qemu.Validate(plan.RuntimeConfig, plan.Layout)
}

func (svc Service) launchPlan(cfg schema.AppConfig) (launchPlan, error) {
	plan, err := svc.resolvePlan(cfg)
	if err != nil {
		return plan, err
	}
	if err := svc.checkManifest(cfg); err != nil {
		return plan, err
	}
	resolved, err := firmware.Resolve(cfg.StartConditions, svc.Paths.UEFIVars(cfg.AppNameID), cfg.ImageMeta.Image)
	if err != nil {
		return plan, err
	}
	plan.Layout.Firmware = resolved.Firmware
	if cfg.StartConditions.TPM {
		plan.Layout.TPMSocket = svc.Paths.TPMSocket(cfg.AppNameID)
	}
	return plan, qemu.Validate(plan.RuntimeConfig, plan.Layout)
}

func (svc Service) networkCommand(plan launchPlan, args []string) ([]string, string, error) {
	if plan.Network != nil {
		return netns.CommandResolved(plan.Authored, args, svc.Paths.Resolv(plan.Authored.AppNameID), *plan.Network, plan.Lookup)
	}
	if len(plan.Authored.RunnerFlags) > 0 {
		// Explicit raw backend mode can add a NIC even when typed intent grants
		// none. Warnings must not describe this invocation as network-isolated.
		return args, "", nil
	}
	return netns.Command(plan.Authored, args, svc.Paths.Resolv(plan.Authored.AppNameID))
}

func (svc Service) Validate(cfg schema.AppConfig) error {
	plan, err := svc.resolvePlan(cfg)
	if err != nil {
		return err
	}
	_, _, err = svc.networkCommand(plan, qemu.PlanArgs(plan.RuntimeConfig, plan.Layout))
	return err
}

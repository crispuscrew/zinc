// Package app sequences VM validation, preparation and execution.
package app

import (
	"fmt"
	"time"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/fs"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/machine"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

const DefaultStopTimeout = 60 * time.Second

type Service struct {
	Store              *fs.Store
	Paths              paths.Paths
	Runtime            machine.Runtime
	Options            vmoptions.Config
	NetworkAttachments []qemu.NetworkAttachment
	Lookup             provision.Lookup
	LoadNetwork        func(schema.AppConfig) (provision.Manifest, error)
	AudioRuntimeDir    string
	// PreparationTimeout bounds initial supervisor readiness. Zero selects
	// DefaultPreparationTimeout; positive overrides are for internal callers/tests.
	PreparationTimeout time.Duration
}

func New(store *fs.Store, layout paths.Paths) Service {
	return Service{Store: store, Paths: layout, Runtime: machine.Runtime{Paths: layout}, LoadNetwork: provision.Load}
}

func (svc Service) Plan(cfg schema.AppConfig) ([]string, string, error) {
	plan, err := svc.launchPlan(cfg)
	if err != nil {
		return nil, "", err
	}
	return svc.networkCommand(plan, qemu.PlanArgs(plan.RuntimeConfig, plan.Layout))
}

func (svc Service) check(cfg schema.AppConfig) error {
	if cfg.Type != schema.ZincVirtualization {
		return fmt.Errorf("app %q is not a VM; use zcr for container apps", cfg.AppNameID)
	}
	if err := validate.Validate(cfg); err != nil {
		return err
	}
	if err := svc.checkDependencies(cfg, nil, map[string]bool{}); err != nil {
		return err
	}
	if err := vmoptions.Validate(svc.Options); err != nil {
		return err
	}
	if svc.Options.AppNameID != cfg.AppNameID || svc.Options.Image != cfg.ImageMeta.Image {
		return fmt.Errorf("runtime options do not match authored app identity/image")
	}
	return nil
}

func (svc Service) layout(cfg schema.AppConfig) qemu.Layout {
	layout := svc.Paths.Layout(cfg.AppNameID, needsProvisioningDisc(cfg, svc.Options))
	layout.Runtime = svc.Options
	return layout
}

func needsProvisioningDisc(cfg schema.AppConfig, runtime vmoptions.Config) bool {
	return cfg.ImageMeta.CloudInit || runtime.Devices == vmoptions.DevicesCompatible
}

func (svc Service) State(name string) (machine.State, error) {
	if err := guestName(name); err != nil {
		return machine.State{}, err
	}
	state, err := svc.Runtime.State(name)
	if err != nil {
		return state, err
	}
	return svc.annotateState(state)
}

func (svc Service) Running() ([]machine.State, error) { return svc.Runtime.Running() }

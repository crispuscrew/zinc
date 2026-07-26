// Package app is the imperative shell of zvr: it sequences a launch (validate, verify the
// pinned base, build the disk and the seed, compose the command line, start the guest)
// over the pure argv builder and the adapters. The order matters and is deliberate -
// nothing is created for an app whose config does not validate, and no guest starts from
// a base image that no longer matches its digest.
package app

import (
	"fmt"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/disk"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/fs"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/machine"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

// DefaultStopTimeout is how long a guest gets to shut itself down cleanly before zvr
// stops waiting. Generous on purpose: a guest flushing disks is doing exactly what the
// graceful path is for, and killing it early is what that path exists to avoid.
const DefaultStopTimeout = 60 * time.Second

// Service is zvr's use cases over the store, the disk builder and the supervisor.
type Service struct {
	Store   *fs.Store
	Paths   paths.Paths
	Runtime machine.Runtime
}

// New wires a service from the resolved paths.
func New(store *fs.Store, layout paths.Paths) Service {
	return Service{Store: store, Paths: layout, Runtime: machine.Runtime{Paths: layout}}
}

// Plan returns the exact command line a launch would run, without touching anything. It
// is what --dry-run prints: the whole point is that an operator can read what their
// config turns into before a guest exists.
func (svc Service) Plan(cfg schema.AppConfig) ([]string, error) {
	if err := svc.check(cfg); err != nil {
		return nil, err
	}
	return qemu.Args(cfg, svc.layout(cfg)), nil
}

// Run boots an app's guest.
func (svc Service) Run(cfg schema.AppConfig) error {
	if err := svc.check(cfg); err != nil {
		return err
	}
	if err := svc.Paths.EnsureDirs(); err != nil {
		return err
	}

	virt := cfg.VirtualizationMeta
	// The digest check happens in here, before a single byte is written: a base that no
	// longer matches what was authorised must stop the launch, not be discovered later.
	if err := disk.EnsureOverlay(cfg.ImageMeta.Image, virt.BaseDigest,
		svc.Paths.Overlay(cfg.AppNameID), virt.DiskSizeGiB); err != nil {
		return err
	}
	if !virt.CloudInit.Disabled {
		// Rebuilt every launch, so an edited identity takes effect without touching the
		// guest's own disk.
		if err := disk.WriteSeed(svc.Paths.Seed(cfg.AppNameID), cfg); err != nil {
			return err
		}
	}
	// Guest Vulkan needs qemu pointed at a venus-capable virglrenderer. Resolved here so a
	// missing one fails the launch with instructions, rather than qemu starting and the
	// guest quietly getting software Vulkan.
	var extraEnv []string
	if cfg.VirtualizationMeta.Vulkan {
		env, err := svc.Paths.VenusEnv()
		if err != nil {
			return err
		}
		extraEnv = env
	}
	return svc.Runtime.Start(cfg.AppNameID, qemu.Args(cfg, svc.layout(cfg)), extraEnv)
}

// Stop shuts a guest down, gracefully unless force is set.
func (svc Service) Stop(name string, force bool, timeout time.Duration) error {
	return svc.Runtime.Stop(name, force, timeout)
}

// State reports one app's guest.
func (svc Service) State(name string) (machine.State, error) { return svc.Runtime.State(name) }

// Running lists live guests.
func (svc Service) Running() ([]machine.State, error) { return svc.Runtime.Running() }

// Reset deletes an app's overlay, returning the guest to the authored base image. This is
// the disposability the design promises: the base is never written to, so everything the
// guest changed lives in the one file this removes.
func (svc Service) Reset(name string) error {
	state, _ := svc.Runtime.State(name)
	if state.Alive {
		return fmt.Errorf("%s is running; stop it before resetting its disk", name)
	}
	overlay := svc.Paths.Overlay(name)
	if err := removeIfPresent(overlay); err != nil {
		return err
	}
	return removeIfPresent(svc.Paths.Seed(name))
}

// check applies the shared schema rules and confirms the app is one zvr owns.
func (svc Service) check(cfg schema.AppConfig) error {
	if cfg.Type != schema.ZincVirtualization {
		return fmt.Errorf("app %q is a container app (Type: %s); run it with zcr", cfg.AppNameID, cfg.Type)
	}
	// Validated at launch as well as at authoring time, because a config can be edited by
	// hand between the two.
	return validate.Validate(cfg)
}

// layout resolves where this app's files live, leaving the seed out when cloud-init is
// disabled so no seed drive is attached.
func (svc Service) layout(cfg schema.AppConfig) qemu.Layout {
	return svc.Paths.Layout(cfg.AppNameID, !cfg.VirtualizationMeta.CloudInit.Disabled)
}

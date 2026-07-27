// Package app is the imperative shell of zvr: it sequences a launch (validate, verify the
// pinned base, build the disk and the seed, compose the command line, start the guest)
// over the pure argv builder and the adapters. The order matters and is deliberate -
// nothing is created for an app whose config does not validate, and no guest starts from
// a base image that no longer matches its digest.
package app

import (
	"fmt"
	"os"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/disk"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/firmware"
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
	layout, err := svc.machineLayout(cfg, false, false)
	if err != nil {
		return nil, err
	}
	return qemu.Args(cfg, layout), nil
}

// Run boots an app's guest.
func (svc Service) Run(cfg schema.AppConfig) error { return svc.start(cfg, false) }

func (svc Service) start(cfg schema.AppConfig, installing bool) error {
	if err := svc.check(cfg); err != nil {
		return err
	}
	// Before anything else. The runtime refuses a second launch, but it does so at the very
	// end, and everything between here and there has side effects on a guest that is
	// already up: rebuilding its seed, and - worse - restarting the TPM emulator its
	// running Windows believes is sealed to this machine.
	if state, _ := svc.Runtime.State(cfg.AppNameID); state.Alive {
		return fmt.Errorf("%s is already running (pid %d)", cfg.AppNameID, state.PID)
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
	if needsProvisioningDisc(virt) {
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
	layout, err := svc.machineLayout(cfg, installing, true)
	if err != nil {
		return err
	}
	return svc.Runtime.Start(cfg.AppNameID, qemu.Args(cfg, layout), extraEnv)
}

// machineLayout resolves the host-side pieces a guest's machine needs before qemu starts:
// its UEFI variable store and, when it has a TPM, a running emulator to back it.
func (svc Service) machineLayout(cfg schema.AppConfig, installing, startServices bool) (qemu.Layout, error) {
	name := cfg.AppNameID
	layout := svc.layout(cfg)
	layout.Installing = installing

	prepared, err := firmware.Prepare(cfg.VirtualizationMeta, svc.Paths.UEFIVars(name), cfg.ImageMeta.Image)
	if err != nil {
		return qemu.Layout{}, err
	}
	layout.Firmware = prepared

	if cfg.VirtualizationMeta.TPM {
		layout.TPMSocket = svc.Paths.TPMSocket(name)
		// A plan must not start anything: --dry-run's whole promise is that it shows what
		// would happen without doing any of it, and a TPM emulator left running would be
		// exactly the sort of side effect that promise rules out.
		if startServices {
			firmware.StopTPM(svc.Paths.TPMSocket(name), svc.Paths.TPMPID(name))
			if _, err := firmware.StartTPM(svc.Paths.TPMState(name), svc.Paths.TPMSocket(name), svc.Paths.TPMPID(name)); err != nil {
				return qemu.Layout{}, err
			}
		}
	}
	return layout, nil
}

// Stop shuts a guest down, gracefully unless force is set. The TPM emulator is a separate
// process, so it has to be stopped with the guest rather than left running against a
// machine that no longer exists.
func (svc Service) Stop(name string, force bool, timeout time.Duration) error {
	err := svc.Runtime.Stop(name, force, timeout)
	firmware.StopTPM(svc.Paths.TPMSocket(name), svc.Paths.TPMPID(name))
	return err
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
	// Everything the guest accumulated, not just its disk. UEFI variables and TPM state are
	// as much "what this guest became" as the filesystem is: leaving them would return a
	// freshly installed disk to a firmware still holding boot entries for the old one, and
	// a TPM holding keys sealed to a machine state that no longer exists. The next run
	// re-seeds both - the firmware from the variables the install left beside the base
	// image, so a reset lands exactly where the install did.
	for _, path := range []string{
		svc.Paths.Overlay(name),
		svc.Paths.Seed(name),
		svc.Paths.UEFIVars(name),
	} {
		if err := removeIfPresent(path); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(svc.Paths.TPMState(name)); err != nil {
		return fmt.Errorf("remove the guest's TPM state: %w", err)
	}
	return nil
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

// layout resolves where this app's files live, leaving the provisioning disc out when the
// guest has no use for it so no extra drive is attached.
func (svc Service) layout(cfg schema.AppConfig) qemu.Layout {
	return svc.Paths.Layout(cfg.AppNameID, needsProvisioningDisc(cfg.VirtualizationMeta))
}

// needsProvisioningDisc reports whether this guest has anything to read off the disc. A
// cloud-init guest reads its identity from it. A guest on the compatible device profile
// reads zinc-setup.cmd from it, which is the only way Zinc can hand such a guest a driver -
// so turning cloud-init off, which a Windows guest reasonably would, must not take the
// script away with it. One predicate for both the build and the attach: two would drift into
// building a disc nobody mounts, or attaching one nobody built.
func needsProvisioningDisc(virt schema.VirtualizationMeta) bool {
	return !virt.CloudInit.Disabled || virt.Devices == schema.VMDevicesCompatible
}

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/firmware"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

func installApp(cfg *schema.AppConfig, name string, flags *flag.FlagSet) error {
	svc, err := service()
	if err != nil {
		return err
	}
	selected, err := loadApp(svc, name)
	if err != nil {
		return err
	}
	if selected.Type != schema.ZincVirtualization {
		return fmt.Errorf("--app must name a VM definition")
	}
	explicit := map[string]bool{}
	flags.Visit(func(option *flag.Flag) { explicit[option.Name] = true })
	if !explicit["memory"] {
		cfg.ResourcesMeta.MaxRamMiB = selected.ResourcesMeta.MaxRamMiB
	}
	if !explicit["vcpus"] {
		cfg.ResourcesMeta.MaxCPUCores = selected.ResourcesMeta.MaxCPUCores
	}
	if !explicit["firmware"] {
		cfg.StartConditions.LoaderBIOS = selected.StartConditions.LoaderBIOS
	}
	if !explicit["secure-boot"] {
		cfg.StartConditions.SecureBoot = selected.StartConditions.SecureBoot
	}
	if !explicit["tpm"] {
		cfg.StartConditions.TPM = selected.StartConditions.TPM
	}
	if !explicit["resolution"] {
		cfg.DisplayMeta.DisplayWidth = selected.DisplayMeta.DisplayWidth
		cfg.DisplayMeta.DisplayHeight = selected.DisplayMeta.DisplayHeight
	}
	cfg.CreatorFlags = selected.CreatorFlags
	cfg.MinimizeFingerprint = selected.MinimizeFingerprint
	return nil
}

func validateInstall(cfg schema.AppConfig, runtime vmoptions.Config) error {
	if err := vmoptions.Path(runtime.Image); err != nil {
		return err
	}
	if runtime.DiskSizeGiB <= 0 || runtime.DiskSizeGiB > (1<<63-1)/(1<<30) {
		return fmt.Errorf("--size must be positive and fit in an int64 byte count")
	}
	if runtime.Devices != vmoptions.DevicesVirtio && runtime.Devices != vmoptions.DevicesCompatible {
		return fmt.Errorf("--devices requires Virtio or Compatible")
	}
	if len(runtime.InstallMedia) > 6 {
		return fmt.Errorf("at most six installation discs are supported")
	}
	seen := map[string]bool{}
	for _, media := range runtime.InstallMedia {
		if err := vmoptions.Path(media); err != nil {
			return err
		}
		if seen[media] || media == runtime.Image {
			return fmt.Errorf("duplicate installation medium or medium equal to target")
		}
		seen[media] = true
		info, err := os.Stat(media)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("installation media must be regular files")
		}
	}
	return qemu.Validate(cfg, qemu.Layout{Runtime: runtime, Installing: true, Overlay: runtime.Image})
}

func planInstall(cfg schema.AppConfig, runtime vmoptions.Config) (qemu.Layout, error) {
	paths, err := paths.Default()
	if err != nil {
		return qemu.Layout{}, err
	}
	layout := qemu.Layout{Overlay: runtime.Image, Runtime: runtime, Installing: true, Identity: runtime.Image,
		PIDFile: paths.PIDFile(cfg.AppNameID), QMP: paths.QMP(cfg.AppNameID), Serial: paths.Serial(cfg.AppNameID)}
	plan, err := firmware.Resolve(cfg.StartConditions, runtime.Image+".uefi-vars.fd", "")
	if err != nil {
		return layout, err
	}
	layout.Firmware = plan.Firmware
	if cfg.StartConditions.TPM {
		layout.TPMSocket = paths.TPMSocket(cfg.AppNameID)
	}
	return layout, qemu.Validate(cfg, layout)
}

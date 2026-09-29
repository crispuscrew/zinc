package qemu

import (
	"fmt"
	"math"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

// Validate checks the combined app/runtime contract, including for an installer
// that has no pinned base yet. App-schema validation remains the caller's job.
func Validate(cfg schema.AppConfig, layout Layout) error {
	if !layout.Installing {
		if err := vmoptions.Validate(layout.Runtime); err != nil {
			return err
		}
		if layout.Runtime.AppNameID != cfg.AppNameID || layout.Runtime.Image != cfg.ImageMeta.Image {
			return fmt.Errorf("VM runtime options do not match this app's AppNameID and ImageMeta.Image")
		}
	}
	cores := cfg.ResourcesMeta.MaxCPUCores
	if cores <= 0 || math.IsNaN(cores) || math.IsInf(cores, 0) || cores != math.Trunc(cores) ||
		cfg.ResourcesMeta.MaxRamMiB <= 0 || cfg.ResourcesMeta.MaxRamMiB > math.MaxInt64/(1<<20) {
		return fmt.Errorf("VM resources require positive RAM and a positive whole CPU count")
	}
	if cfg.StartConditions.LoaderBIOS && cfg.StartConditions.SecureBoot {
		return fmt.Errorf("StartConditions.SecureBoot requires UEFI (LoaderBIOS: false)")
	}
	display := ResolveDisplay(cfg, layout.Runtime)
	if cfg.StopConditions.Background && display != vmoptions.DisplayNone {
		return fmt.Errorf("StopConditions.Background requires serial/headless presentation; closing an embedded QEMU graphics window terminates QEMU")
	}
	if cfg.StartConditions.Terminal && display != vmoptions.DisplayNone {
		return fmt.Errorf("StartConditions.Terminal requires runtime Display None; a graphical QEMU window is not a terminal session")
	}
	if display == vmoptions.DisplayAccelerated && cfg.DisplayMeta.DisableGpuAccess {
		return fmt.Errorf("runtime Display: Accelerated conflicts with DisplayMeta.DisableGpuAccess")
	}
	if cfg.DisplayMeta.Vulkan && display != vmoptions.DisplayAccelerated {
		return fmt.Errorf("DisplayMeta.Vulkan requires the Accelerated display")
	}
	width, height := cfg.DisplayMeta.DisplayWidth, cfg.DisplayMeta.DisplayHeight
	if width != 0 || height != 0 {
		if width < 640 || height < 480 || width%2 != 0 {
			return fmt.Errorf("DisplayMeta.DisplayWidth/DisplayHeight require an even width and at least 640x480")
		}
		if _, valid := schema.GuestDisplay(width, height); !valid {
			return fmt.Errorf("DisplayMeta resolution %dx%d exceeds EDID limits", width, height)
		}
		if display != vmoptions.DisplayCompatible || cfg.StartConditions.LoaderBIOS {
			return fmt.Errorf("fixed display resolution requires Compatible display and UEFI")
		}
	}
	if cfg.StartConditions.ReadOnlyRootfs && (layout.Installing || cfg.ImageMeta.CloudInit) {
		return fmt.Errorf("ReadOnlyRootfs cannot be combined with installation or writable cloud-init provisioning")
	}
	for _, path := range []string{layout.Overlay, layout.Seed, layout.PIDFile, layout.QMP, layout.Serial,
		layout.TPMSocket, layout.Firmware.CodePath, layout.Firmware.VarsPath} {
		if path != "" {
			if err := vmoptions.Path(path); err != nil {
				return err
			}
		}
	}
	for _, socket := range []string{layout.QMP, layout.Serial, layout.TPMSocket} {
		if len(socket) > 107 {
			return fmt.Errorf("Unix socket path is longer than 107 bytes: %s", socket)
		}
	}
	for _, flags := range [][]string{cfg.RunnerFlags, cfg.CreatorFlags} {
		for _, value := range flags {
			if strings.ContainsRune(value, 0) {
				return fmt.Errorf("raw backend arguments cannot contain NUL")
			}
		}
	}
	if err := validateNetwork(cfg, layout); err != nil {
		return err
	}
	return validateAudio(cfg.AudioMeta, layout.Audio)
}

func Warnings(cfg schema.AppConfig, runtime vmoptions.Config, installing bool) []string {
	var warnings []string
	flags, field := cfg.RunnerFlags, "RunnerFlags"
	if installing {
		flags, field = cfg.CreatorFlags, "CreatorFlags"
	}
	if len(flags) > 0 {
		warnings = append(warnings, field+": raw QEMU arguments can override isolation, disks and control sockets; typed policy guarantees no longer apply")
	}
	if cfg.DisplayMeta.Vulkan {
		warnings = append(warnings, "DisplayMeta.Vulkan: QEMU's seccomp sandbox is disabled for the Venus renderer")
	}
	display := ResolveDisplay(cfg, runtime)
	if runtime.Devices == vmoptions.DevicesCompatible && (display == vmoptions.DisplayWindow || display == vmoptions.DisplayAccelerated) {
		warnings = append(warnings, "virtio display requires a guest graphics driver even with Compatible disk/network devices")
	}
	if cfg.MinimizeFingerprint && cfg.InternalUserMeta.KeepUserID {
		warnings = append(warnings, "KeepUserID preserves host identity despite MinimizeFingerprint")
	}
	if len(cfg.RunnerFlags) > 0 && len(cfg.NetworkMeta.Interfaces) == 0 && !installing {
		warnings = append(warnings, "empty Interfaces grants no managed NIC, but raw RunnerFlags may add networking; this run is not guaranteed network-isolated")
	}
	for _, forward := range runtime.ForwardPorts {
		warnings = append(warnings, fmt.Sprintf("publishing %s %s:%d to guest port %d", forward.Transport(), forward.Bind(), forward.HostPort, forward.GuestPort))
	}
	return warnings
}

package validate

import "github.com/crispuscrew/zinc/common/domain/schema"

// checkDisplay screens DisplayMeta.
//
// The two security-context flags are opposites: one says "run me without one", the other says
// "refuse to run me without one". A config asserting both has not narrowed anything, it has
// said two incompatible things, and picking a winner would mean one of them silently doing
// nothing. Refuse instead, the same way the schema refuses every other ambiguous pairing.
func checkDisplay(cfg schema.AppConfig, add addFunc) {
	display := cfg.DisplayMeta
	if display.DisableSecurityContext && display.RequireSecurityContext {
		add("DisplayMeta: DisableSecurityContext and RequireSecurityContext are opposites - one runs the app without a security context, the other refuses to run it without one; set at most one")
	}
	if display.DisableGpuAccess && display.Vulkan {
		add("DisplayMeta.Vulkan: requires GPU access (DisableGpuAccess must be false)")
	}
	width, height := display.DisplayWidth, display.DisplayHeight
	if width == 0 && height == 0 {
		return
	}
	if width == 0 || height == 0 {
		add("DisplayMeta.DisplayWidth/DisplayHeight %dx%d: set both or neither", width, height)
		return
	}
	if width < 0 || height < 0 {
		add("DisplayMeta.DisplayWidth/DisplayHeight %dx%d: dimensions must be positive", width, height)
		return
	}
	if cfg.Type != schema.ZincVirtualization {
		return
	}
	checkGuestResolution(width, height, cfg.StartConditions.LoaderBIOS, add)
}

// EDID limits belong to the emulated guest display, not a host compositor.
func checkGuestResolution(width, height int, bios bool, add addFunc) {
	if bios {
		add("DisplayMeta.DisplayWidth/DisplayHeight: a fixed guest mode requires UEFI (StartConditions.LoaderBIOS must be false)")
	}
	if width < 640 || height < 480 {
		add("DisplayMeta.DisplayWidth/DisplayHeight %dx%d: must be at least 640x480", width, height)
	}
	if width%2 != 0 {
		add("DisplayMeta.DisplayWidth %d: must be even", width)
	}
	if _, valid := schema.GuestDisplay(width, height); !valid && width >= 640 && height >= 480 {
		add("DisplayMeta.DisplayWidth/DisplayHeight %dx%d: too large for the display EDID", width, height)
	}
}

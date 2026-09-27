package qemu

import (
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

const hostMemGiB = 8

func displayArgs(cfg schema.AppConfig, runtime vmoptions.Config) []string {
	switch ResolveDisplay(cfg, runtime) {
	case vmoptions.DisplayCompatible:
		return append(inputArgs(runtime.Devices), "-device", compatibleDisplayDevice(cfg), "-display", "gtk")
	case vmoptions.DisplayAccelerated:
		device := "virtio-gpu-gl-pci"
		if cfg.DisplayMeta.Vulkan {
			device += fmt.Sprintf(",venus=on,blob=on,hostmem=%dG", hostMemGiB)
		}
		return append(inputArgs(runtime.Devices), "-device", device, "-display", "gtk,gl=on")
	case vmoptions.DisplayWindow:
		return append(inputArgs(runtime.Devices), "-device", "virtio-gpu-pci", "-display", "gtk")
	default:
		return []string{"-display", "none"}
	}
}

func compatibleDisplayDevice(cfg schema.AppConfig) string {
	display := cfg.DisplayMeta
	mode, valid := schema.GuestDisplay(display.DisplayWidth, display.DisplayHeight)
	if cfg.StartConditions.LoaderBIOS || !valid {
		return "VGA,vgamem_mb=64"
	}
	return fmt.Sprintf("bochs-display,xres=%d,yres=%d,vgamem=%d,refresh_rate=%d",
		display.DisplayWidth, display.DisplayHeight, mode.VideoMemBytes, mode.RefreshMilliHz)
}

func inputArgs(devices vmoptions.Devices) []string {
	if devices == vmoptions.DevicesCompatible {
		return []string{"-device", "qemu-xhci,id=usb", "-device", "usb-tablet,bus=usb.0", "-device", "usb-kbd,bus=usb.0"}
	}
	return []string{"-device", "virtio-keyboard-pci", "-device", "virtio-tablet-pci"}
}

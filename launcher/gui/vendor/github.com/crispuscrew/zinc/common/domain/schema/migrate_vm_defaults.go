package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (change *migration) migrateVMDefaults(defaults bool) error {
	firmware := change.value("VirtualizationMeta.Firmware")
	if firmware != nil || defaults {
		var mode string
		if firmware != nil {
			if err := firmware.Decode(&mode); err != nil {
				return fmt.Errorf("VirtualizationMeta.Firmware: %w", err)
			}
		}
		if mode != "" && mode != "BIOS" && mode != "UEFI" {
			return fmt.Errorf("VirtualizationMeta.Firmware %q: choose BIOS or UEFI explicitly", mode)
		}
		if firmware != nil || change.value("StartConditions.LoaderBIOS") == nil {
			if err := change.putValue("StartConditions.LoaderBIOS", mode != "UEFI"); err != nil {
				return fmt.Errorf("VirtualizationMeta.Firmware: %w", err)
			}
		}
		change.remove("VirtualizationMeta.Firmware")
	}
	cloud := change.value("VirtualizationMeta.CloudInit")
	if cloud != nil && cloud.Tag != "!!null" && cloud.Kind != yaml.MappingNode {
		return fmt.Errorf("VirtualizationMeta.CloudInit must be a mapping")
	}
	disabled := change.value("VirtualizationMeta.CloudInit.Disabled")
	if disabled != nil || defaults || cloud != nil && cloud.Tag == "!!null" {
		var flag bool
		if disabled != nil {
			if err := disabled.Decode(&flag); err != nil {
				return fmt.Errorf("VirtualizationMeta.CloudInit.Disabled: %w", err)
			}
		}
		if disabled != nil || cloud != nil || change.value("ImageMeta.CloudInit") == nil {
			if err := change.putValue("ImageMeta.CloudInit", !flag); err != nil {
				return fmt.Errorf("VirtualizationMeta.CloudInit.Disabled: %w", err)
			}
		}
		change.remove("VirtualizationMeta.CloudInit.Disabled")
	}
	return nil
}

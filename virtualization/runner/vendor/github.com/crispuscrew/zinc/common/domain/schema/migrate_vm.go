package schema

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func (change *migration) migrateVM(version int) error {
	var appType Type
	if node := change.value("Type"); node != nil {
		if err := node.Decode(&appType); err != nil {
			return fmt.Errorf("Type: %w", err)
		}
	}
	virtualization := change.value("VirtualizationMeta")
	if virtualization != nil && virtualization.Tag != "!!null" && virtualization.Kind != yaml.MappingNode {
		return fmt.Errorf("VirtualizationMeta must be a mapping")
	}
	if appType == ZincContainer {
		return change.removeContainerVM()
	}
	if appType != "" && appType != ZincVirtualization && virtualization != nil {
		return fmt.Errorf("VirtualizationMeta requires Type: ZincVirtualization")
	}
	for _, names := range [][2]string{{"MemoryMiB", "MaxRamMiB"}, {"VCPUs", "MaxCPUCores"}} {
		source, target := "VirtualizationMeta."+names[0], "ResourcesMeta."+names[1]
		if value := change.value(source); value != nil {
			if appType == "" && legacyZero(value, "number") {
				change.remove(source)
				continue
			}
			// Generated v3 files contained zero container resource placeholders.
			if existing := change.value(target); version == previousSchemaVersion && existing != nil && legacyZero(existing, "number") {
				change.remove(target)
			}
			if err := change.move(source, target); err != nil {
				return err
			}
		}
	}
	for _, names := range [][2]string{
		{"DisplayWidth", "DisplayMeta.DisplayWidth"}, {"DisplayHeight", "DisplayMeta.DisplayHeight"},
		{"Vulkan", "DisplayMeta.Vulkan"}, {"SecureBoot", "StartConditions.SecureBoot"},
		{"TPM", "StartConditions.TPM"}, {"CloudInit.SSHKeyPath", "ImageMeta.PublicSSHKeyPath"},
	} {
		if err := change.move("VirtualizationMeta."+names[0], names[1]); err != nil {
			return err
		}
	}
	// Inheritance fragments only contribute stated settings. A complete recognized
	// old VM needs the old BIOS/cloud-init defaults made explicit in the new model.
	var parent string
	if inherits := change.value("Inherits"); inherits != nil {
		if err := inherits.Decode(&parent); err != nil {
			return fmt.Errorf("Inherits: %w", err)
		}
	}
	defaults := appType == ZincVirtualization && strings.TrimSpace(parent) == ""
	if err := change.migrateVMDefaults(defaults); err != nil {
		return err
	}
	for _, field := range legacyVMRuntimeFields {
		if err := change.removeZero("VirtualizationMeta."+field[0], field[1]); err != nil {
			return err
		}
	}
	if err := change.removeEmpty("VirtualizationMeta.CloudInit"); err != nil {
		return err
	}
	return change.removeEmpty("VirtualizationMeta")
}

var legacyVMRuntimeFields = [][2]string{
	{"BaseDigest", "string"}, {"DiskSizeGiB", "number"}, {"Display", "string"},
	{"Devices", "string"}, {"InstallMedia", "list"}, {"ForwardPorts", "list"},
	{"MacAddress", "string"}, {"CloudInit.UserName", "string"},
}

func (change *migration) removeContainerVM() error {
	fields := append([][2]string{}, legacyVMRuntimeFields...)
	fields = append(fields, [][2]string{
		{"MemoryMiB", "number"}, {"VCPUs", "number"}, {"DisplayWidth", "number"},
		{"DisplayHeight", "number"}, {"Vulkan", "bool"}, {"Firmware", "string"},
		{"SecureBoot", "bool"}, {"TPM", "bool"}, {"CloudInit.Disabled", "bool"},
		{"CloudInit.SSHKeyPath", "string"},
	}...)
	for _, field := range fields {
		if err := change.removeZero("VirtualizationMeta."+field[0], field[1]); err != nil {
			return err
		}
	}
	if err := change.removeEmpty("VirtualizationMeta.CloudInit"); err != nil {
		return err
	}
	return change.removeEmpty("VirtualizationMeta")
}

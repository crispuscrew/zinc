package tui

import (
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func (frm *formModel) sharedFields() []formField {
	cfg := frm.draft
	frm.name = newInput(cfg.AppNameID, "app-name")
	identity := formField{label: "name", kind: kindInfo, info: func() string { return frm.draft.AppNameID }}
	if frm.creating {
		identity = formField{label: "name", kind: kindText, input: &frm.name}
	}
	fields := []formField{identity, {
		label: "type", kind: kindEnum, rebuild: true,
		values: []string{string(schema.ZincContainer), string(schema.ZincVirtualization)},
		get:    func() string { return string(frm.draft.Type) },
		set:    func(value string) { frm.draft.Type = schema.Type(value); frm.seedVMResources() },
	}}
	fields = append(fields,
		frm.text("image", &frm.image, cfg.ImageMeta.Image, func(cfg *schema.AppConfig, value string) error {
			cfg.ImageMeta.Image = strings.TrimSpace(value)
			return nil
		}),
		frm.text("description", &frm.desc, cfg.LauncherMeta.Description, func(cfg *schema.AppConfig, value string) error { cfg.LauncherMeta.Description = value; return nil }),
		frm.text("icon", &frm.icon, cfg.LauncherMeta.Icon, func(cfg *schema.AppConfig, value string) error { cfg.LauncherMeta.Icon = value; return nil }),
		frm.text("group", &frm.group, cfg.LauncherMeta.Group, func(cfg *schema.AppConfig, value string) error { cfg.LauncherMeta.Group = value; return nil }),
		frm.multiline("install", &frm.install, strings.Join(cfg.ImageMeta.Install, "\n"), func(cfg *schema.AppConfig, value string) error { cfg.ImageMeta.Install = splitLines(value); return nil }),
		frm.scalar("source tag", cfg.ImageMeta.SourceTag, func(cfg *schema.AppConfig, value string) error { cfg.ImageMeta.SourceTag = value; return nil }),
		frm.scalar("inherits (file edit)", cfg.Inherits, func(cfg *schema.AppConfig, value string) error { cfg.Inherits = value; return nil }),
	)
	fields = append(fields, frm.lifecycleFields()...)
	fields = append(fields, frm.resourceFields()...)
	fields = append(fields, frm.displayFields()...)
	fields = append(fields, frm.audioFields()...)
	fields = append(fields, frm.complexFields()...)
	return fields
}

func (frm *formModel) resourceFields() []formField {
	cfg := frm.draft
	fields := []formField{
		frm.text("memory (MiB)", &frm.memory, numText(cfg.ResourcesMeta.MaxRamMiB, 0), func(cfg *schema.AppConfig, value string) error {
			number, err := wholeNumber(value)
			cfg.ResourcesMeta.MaxRamMiB = number
			return err
		}),
		frm.text("cpus", &frm.vcpus, cpuText(cfg.ResourcesMeta.MaxCPUCores, 0), func(cfg *schema.AppConfig, value string) error {
			number, err := cpuNumber(value)
			cfg.ResourcesMeta.MaxCPUCores = number
			return err
		}),
		frm.scalar("pids limit", numText(cfg.ResourcesMeta.PIDsLimit, 0), func(cfg *schema.AppConfig, value string) error {
			number, err := wholeNumber(value)
			cfg.ResourcesMeta.PIDsLimit = number
			return err
		}),
		frm.text("user name", &frm.ciUser, cfg.InternalUserMeta.NonRootUserName, func(cfg *schema.AppConfig, value string) error {
			cfg.InternalUserMeta.NonRootUserName = value
			return nil
		}),
		toggle("non-root user", func() bool { return frm.draft.InternalUserMeta.UseNonRootUser }, func(value bool) { frm.draft.InternalUserMeta.UseNonRootUser = value }),
		toggle("keep user ID", func() bool { return frm.draft.InternalUserMeta.KeepUserID }, func(value bool) { frm.draft.InternalUserMeta.KeepUserID = value }),
		toggle("minimize fingerprint", func() bool { return frm.draft.MinimizeFingerprint }, func(value bool) { frm.draft.MinimizeFingerprint = value }),
		toggle("host_theme", func() bool { return frm.draft.HostTheme }, func(value bool) { frm.draft.HostTheme = value }),
	}
	frm.seedVMResources()
	return fields
}

func (frm *formModel) seedVMResources() {
	if !frm.creating || frm.draft.Type != schema.ZincVirtualization {
		return
	}
	if frm.memory.Value() == "" {
		frm.memory.SetValue("4096")
	}
	if frm.vcpus.Value() == "" {
		frm.vcpus.SetValue("2")
	}
}

func (frm *formModel) displayFields() []formField {
	display := frm.draft.DisplayMeta
	return []formField{
		toggle("display.disable_gpu", func() bool { return frm.draft.DisplayMeta.DisableGpuAccess }, func(value bool) { frm.draft.DisplayMeta.DisableGpuAccess = value }),
		toggle("display.disable_security_context", func() bool { return frm.draft.DisplayMeta.DisableSecurityContext }, func(value bool) { frm.draft.DisplayMeta.DisableSecurityContext = value }),
		toggle("display.require_security_context", func() bool { return frm.draft.DisplayMeta.RequireSecurityContext }, func(value bool) { frm.draft.DisplayMeta.RequireSecurityContext = value }),
		toggle("display.vulkan", func() bool { return frm.draft.DisplayMeta.Vulkan }, func(value bool) { frm.draft.DisplayMeta.Vulkan = value }),
		frm.scalar("display.width", strconv.Itoa(display.DisplayWidth), func(cfg *schema.AppConfig, value string) error {
			number, err := strconv.Atoi(value)
			cfg.DisplayMeta.DisplayWidth = number
			return err
		}),
		frm.scalar("display.height", strconv.Itoa(display.DisplayHeight), func(cfg *schema.AppConfig, value string) error {
			number, err := strconv.Atoi(value)
			cfg.DisplayMeta.DisplayHeight = number
			return err
		}),
	}
}

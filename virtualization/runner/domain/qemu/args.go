// Package qemu builds argv from validated app intent and resolved host inputs.
package qemu

import (
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

const Binary = "qemu-system-x86_64"

type Layout struct {
	Overlay, Seed, PIDFile, QMP, Serial string
	Firmware                            Firmware
	TPMSocket                           string
	Installing                          bool
	Identity                            string
	Namespaced                          bool
	Runtime                             vmoptions.Config
	NetworkAttachments                  []NetworkAttachment
	Audio                               AudioLayout
}

// NetworkAttachment connects a logical NIC to a TAP already provisioned in the
// namespace QEMU will enter. QEMU never runs host network setup scripts.
type NetworkAttachment struct {
	InterfaceID string
	TapName     string
}

type Firmware struct {
	CodePath string
	VarsPath string
	Format   string
}

func ResolveDisplay(cfg schema.AppConfig, runtime vmoptions.Config) vmoptions.Display {
	if runtime.Display != "" {
		return runtime.Display
	}
	if cfg.StartConditions.Terminal {
		return vmoptions.DisplayNone
	}
	if cfg.DisplayMeta.DisableGpuAccess {
		return vmoptions.DisplayCompatible
	}
	return vmoptions.DisplayAccelerated
}

// Args requires Validate to have succeeded. Raw backend flags deliberately remain
// an escape hatch; callers warn before execution and never shell-evaluate them.
func Args(cfg schema.AppConfig, layout Layout) []string {
	args := []string{Binary, "-name", cfg.AppNameID,
		"-machine", machineType(cfg.StartConditions), "-cpu", "host",
		"-smp", strconv.FormatFloat(cfg.ResourcesMeta.MaxCPUCores, 'f', -1, 64),
		"-m", strconv.FormatInt(cfg.ResourcesMeta.MaxRamMiB, 10) + "M", "-nodefaults",
		"-pidfile", layout.PIDFile, "-qmp", "unix:" + layout.QMP + ",server=on,wait=off",
		"-serial", "unix:" + layout.Serial + ",server=on,wait=off"}
	identity := cfg.AppNameID
	if layout.Identity != "" {
		identity = layout.Identity
	}
	args = append(args, identityArgs(identity, cfg.MinimizeFingerprint)...)
	if cfg.StopConditions.KeepAlive {
		args = append(args, "-no-shutdown")
	}
	args = append(args, secureBootArgs(cfg.StartConditions)...)
	args = append(args, rtcArgs(layout.Runtime.Devices)...)
	if !cfg.DisplayMeta.Vulkan {
		args = append(args, "-sandbox", "on,obsolete=deny,elevateprivileges=deny,spawn=deny,resourcecontrol=deny")
	}
	args = append(args, firmwareArgs(layout.Firmware)...)
	args = append(args, tpmArgs(layout.TPMSocket)...)
	args = append(args, diskArgs(cfg, layout)...)
	args = append(args, mediaArgs(layout.Runtime.InstallMedia)...)
	if layout.Installing {
		args = append(args, "-boot", "once=d,menu=on")
	}
	args = append(args, networkArgs(cfg, layout, identity)...)
	args = append(args, displayArgs(cfg, layout.Runtime)...)
	args = append(args, audioArgs(layout.Audio)...)
	if layout.Installing {
		return append(args, cfg.CreatorFlags...)
	}
	return append(args, cfg.RunnerFlags...)
}

// PlanArgs omits raw backend values: they can contain credentials. A plan shows
// typed settings and reports the omitted override count separately.
func PlanArgs(cfg schema.AppConfig, layout Layout) []string {
	cfg.CreatorFlags, cfg.RunnerFlags = nil, nil
	return Args(cfg, layout)
}

// Display is shell-copyable even when raw arguments contain metacharacters.
func Display(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

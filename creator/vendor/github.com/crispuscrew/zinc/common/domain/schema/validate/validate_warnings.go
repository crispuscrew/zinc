package validate

import (
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Warnings reports explicit risky grants without changing validation or policy.
func Warnings(cfg schema.AppConfig) []string {
	var warnings []string
	for _, flags := range []struct {
		field  string
		values []string
	}{{"CreatorFlags", cfg.CreatorFlags}, {"RunnerFlags", cfg.RunnerFlags}} {
		if len(flags.values) > 0 {
			warnings = append(warnings, flags.field+": unsafe raw backend arguments explicitly chosen by the user; they can override Zinc's isolation and structured policy")
		}
	}
	if cfg.Type == schema.ZincVirtualization && cfg.DisplayMeta.Vulkan {
		warnings = append(warnings, "DisplayMeta.Vulkan: guest Vulkan disables qemu's seccomp sandbox because the renderer needs a helper process")
	}
	warnings = append(warnings, dbusWarnings(cfg.DBusMeta)...)
	warnings = append(warnings, networkWarnings(cfg.NetworkMeta)...)
	if cfg.MinimizeFingerprint {
		warnings = append(warnings, fingerprintWarnings(cfg)...)
	}
	return warnings
}

func networkWarnings(network schema.NetworkMeta) []string {
	var warnings []string
	if len(network.Interfaces) == 0 && (len(network.RulesByPriority) > 0 || len(network.DNS.ResolversByPriority) > 0) {
		warnings = append(warnings, "NetworkMeta.Interfaces: empty means no external NIC; rules and DNS do not create one")
	}
	if len(network.Interfaces) > 0 && len(network.DNS.ResolversByPriority) == 0 {
		warnings = append(warnings, "NetworkMeta.DNS: no explicit resolvers; host DNS is never used implicitly, so external names cannot resolve")
	}
	for index, rule := range network.RulesByPriority {
		if !rule.AllowAllExcept && rule.To.Type == schema.NetworkPeerSelf && rule.From.Type != schema.NetworkPeerSelf {
			warnings = append(warnings, fmt.Sprintf("NetworkMeta.RulesByPriority[%d]: allows new inbound connections from %s, subject to earlier first-match rules", index, rule.From.Type))
		}
		if len(rule.Domains) > 0 {
			warnings = append(warnings, fmt.Sprintf("NetworkMeta.RulesByPriority[%d].Domains: matches resolved IP addresses, not application hostnames; shared addresses can serve other names", index))
		}
	}
	return warnings
}

func fingerprintWarnings(cfg schema.AppConfig) []string {
	var warnings []string
	for _, conflict := range []struct {
		set   bool
		field string
	}{
		{cfg.DisplayMeta.Vulkan, "DisplayMeta.Vulkan"},
		{cfg.DisplayMeta.DisplayWidth != 0 || cfg.DisplayMeta.DisplayHeight != 0, "DisplayMeta.DisplayWidth/DisplayHeight"},
		{cfg.InternalUserMeta.KeepUserID, "InternalUserMeta.KeepUserID"},
		{cfg.InternalUserMeta.NonRootUserName != "", "InternalUserMeta.NonRootUserName"},
		{cfg.HostTheme, "HostTheme"},
		{len(cfg.CreatorFlags) > 0, "CreatorFlags"},
		{len(cfg.RunnerFlags) > 0, "RunnerFlags"},
		{len(cfg.AudioMeta.Playback.PipeWireDevices)+len(cfg.AudioMeta.Playback.ALSADevices)+
			len(cfg.AudioMeta.Microphone.PipeWireDevices)+len(cfg.AudioMeta.Microphone.ALSADevices)+
			len(cfg.AudioMeta.Monitor.PipeWireDevices)+len(cfg.AudioMeta.Monitor.ALSADevices) > 0, "AudioMeta device selectors"},
	} {
		if conflict.set {
			warnings = append(warnings, "MinimizeFingerprint: "+conflict.field+" can expose stable host or custom configuration details; minimization is best-effort")
		}
	}
	for index, iface := range cfg.NetworkMeta.Interfaces {
		if iface.MacAddress != "" {
			warnings = append(warnings, fmt.Sprintf("MinimizeFingerprint: NetworkMeta.Interfaces[%d].MacAddress fixes a stable network identifier", index))
		}
	}
	return warnings
}

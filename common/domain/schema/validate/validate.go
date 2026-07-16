package validate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Validate checks an AppConfig against the hard rules. Pure (no I/O), so zcc (save)
// and zcr (launch) judge identically; all problems are joined, not just the first.
func Validate(cfg schema.AppConfig) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	checkIdentity(cfg, add)
	checkLifecycle(cfg, add)
	checkResources(cfg.ResourcesMeta, add)

	for index, netList := range cfg.NetworkMeta.NetworkLists {
		checkNetworkList(index, netList, add)
	}
	for index, volume := range cfg.Volumes {
		checkVolume(index, volume, add)
	}
	for index, configMount := range cfg.Configs {
		checkConfig(index, configMount, add)
	}
	checkKeys(cfg.Keys, add)
	checkCapabilities(cfg.Capabilities, add)

	return errors.Join(errs...)
}

// checkIdentity: schema version, Type, AppNameID, the digest-pinned image, user name.
func checkIdentity(cfg schema.AppConfig, add addFunc) {
	if cfg.SchemaVersion != schema.SchemaVersion {
		add("SchemaVersion: got %d, want %d", cfg.SchemaVersion, schema.SchemaVersion)
	}

	switch cfg.Type {
	case schema.ZincContainer:
	case "":
		add("Type: must be set (ZincContainer)")
	default:
		add("Type %q: must be ZincContainer (ZincVirtualization not yet supported)", cfg.Type)
	}

	switch {
	case strings.TrimSpace(cfg.AppNameID) == "":
		add("AppNameID: must not be empty")
	case !nameRE.MatchString(cfg.AppNameID):
		add("AppNameID %q: only lowercase [a-z0-9._-] allowed, must start alphanumeric", cfg.AppNameID)
	}

	switch {
	case strings.TrimSpace(cfg.ImageMeta.Image) == "":
		add("ImageMeta.Image: must not be empty")
	case hasUnsafe(cfg.ImageMeta.Image):
		// Interpolated into a FROM line - must be a single-line ref (section 5.5).
		add("ImageMeta.Image %q: must be a single-line reference (no whitespace or control characters)", cfg.ImageMeta.Image)
	case !LocalImage(cfg.ImageMeta.Image) && !digestRE.MatchString(cfg.ImageMeta.Image):
		// section 5.5: third-party images pinned by canonical digest; only localhost/ may use a mutable tag.
		add("ImageMeta.Image %q: third-party images must be digest-pinned (...@sha256:<64 hex>); only localhost/ images may use a mutable tag (section 5.5)", cfg.ImageMeta.Image)
	}

	// NonRootUserName becomes `podman --user`; keep it a safe charset.
	if name := cfg.InternalUserMeta.NonRootUserName; name != "" && !nameRE.MatchString(name) {
		add("InternalUserMeta.NonRootUserName %q: only lowercase [a-z0-9._-] allowed, must start alphanumeric", name)
	}
}

// checkLifecycle: terminal / multiterminal / background interplay (section 9.1).
func checkLifecycle(cfg schema.AppConfig, add addFunc) {
	start := cfg.StartConditions
	switch {
	case start.Multiterminal && !start.Terminal:
		add("StartConditions.Multiterminal: requires Terminal (it spawns terminals into a shared container)")
	case start.Terminal && cfg.StopConditions.Background && !start.Multiterminal:
		// A foreground terminal app can't also be Background; Multiterminal lifts this.
		add("StartConditions.Terminal: a terminal app runs in a foreground window; it cannot also be StopConditions.Background (use Multiterminal to keep the shared container alive after the last terminal closes)")
	}
	if start.Multiterminal && strings.TrimSpace(start.Entrypoint) == "" && strings.TrimSpace(start.MultiterminalEntrypoint) == "" {
		// Each terminal re-execs the app, so it needs an explicit command (PID 1 is a holder).
		add("StartConditions: Multiterminal needs an explicit Entrypoint or MultiterminalEntrypoint (the image default cannot be replayed into each terminal)")
	}
}

// checkResources: caps are >= 0 (0 = unlimited for CPU/RAM/PIDs).
func checkResources(res schema.ResourcesMeta, add addFunc) {
	if res.MaxCPUCores < 0 {
		add("ResourcesMeta.MaxCPUCores %v: must be >= 0 (0 = unlimited)", res.MaxCPUCores)
	}
	if res.MaxRamMiB < 0 {
		add("ResourcesMeta.MaxRamMiB %d: must be >= 0 (0 = unlimited)", res.MaxRamMiB)
	}
	if res.MaxSwapMiB < 0 {
		add("ResourcesMeta.MaxSwapMiB %d: must be >= 0", res.MaxSwapMiB)
	}
	if res.PIDsLimit < 0 {
		add("ResourcesMeta.PIDsLimit %d: must be >= 0 (0 = unlimited)", res.PIDsLimit)
	}
}

// Warnings returns non-fatal create-time advisories (zcc); nothing here blocks save or
// launch - it flags valid-but-risky or probably-unintended configs. Exposing inbound
// (an Ingress list) is always surfaced, loudest when it reaches the LAN.
func Warnings(cfg schema.AppConfig) []string {
	var warns []string
	for index, netList := range cfg.NetworkMeta.NetworkLists {
		if netList.Ingress {
			warns = append(warns, ingressWarnings(index, netList)...)
			continue
		}
		// Egress: an empty blacklist blocks nothing (allow-all) - worth surfacing on a
		// security tool.
		if netList.Blacklist &&
			len(netList.IPv4CIDR) == 0 && len(netList.IPv6CIDR) == 0 && len(netList.Ports) == 0 {
			warns = append(warns, fmt.Sprintf(
				"NetworkLists[%d]: egress blacklist with no CIDRs/ports blocks nothing (allow-all)", index))
		}
	}
	return warns
}

// ingressWarnings surfaces one published-port (Ingress) list: inbound exposure always
// gets a notice, and the loud form when it reaches the LAN (Host) or opens every port
// (an ingress blacklist = default-accept inbound). A ports-less ingress list exposes
// nothing and most likely means the author forgot Ports.
func ingressWarnings(index int, netList schema.NetworkList) []string {
	scope := "apps that join this app's network"
	if netList.Host {
		iface := strings.TrimSpace(netList.Interface)
		if iface == "" {
			iface = "all host interfaces"
		}
		scope = fmt.Sprintf("the LAN via %s", iface)
	}
	switch {
	case netList.Blacklist:
		return []string{fmt.Sprintf(
			"NetworkLists[%d]: ingress blacklist exposes ALL inbound ports (default-accept) to %s", index, scope)}
	case len(netList.Ports) > 0:
		return []string{fmt.Sprintf(
			"NetworkLists[%d]: ingress exposes port(s) %s to %s", index, joinPorts(netList.Ports), scope)}
	default:
		return []string{fmt.Sprintf(
			"NetworkLists[%d]: ingress list exposes no ports (Ports is empty) - did you forget Ports? (%s)", index, scope)}
	}
}

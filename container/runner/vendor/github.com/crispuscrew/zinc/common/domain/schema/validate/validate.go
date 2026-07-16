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
	checkInstall(cfg.ImageMeta.Install, add)
	checkDepends(cfg.StartConditions.DependsOn, add)

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
	checkNetworkCapabilities(cfg, add)

	return errors.Join(errs...)
}

// checkInstall screens each ImageMeta.Install step. The steps are joined into the one
// RUN layer of the derived-image Containerfile (FROM ImageMeta.Image + RUN ...), so a
// control character - above all a newline - would break out of that single RUN line
// and let a crafted config inject its own Containerfile directives (e.g. a second FROM
// that swaps the base to an unpinned image, defeating the digest pin while the YAML's
// Image still looks pinned). A legitimate multi-step setup uses one list entry per
// step; a single step never needs an embedded newline (section 5.5).
func checkInstall(install []string, add addFunc) {
	for index, step := range install {
		if hasControl(step) {
			add("ImageMeta.Install[%d]: must not contain control characters (a newline would inject extra Containerfile directives); put each setup step in its own list entry (section 5.5)", index)
		}
	}
}

// checkDepends screens each StartConditions.DependsOn name. A dependency name is used
// verbatim to locate its app file on disk (the store joins it into a path), so it must
// be a safe object name - the same charset as AppNameID - or a "../.." value could
// read and launch a definition from outside the apps directory (section 6.6).
func checkDepends(dependsOn []string, add addFunc) {
	for index, dep := range dependsOn {
		if strings.TrimSpace(dep) == "" {
			add("StartConditions.DependsOn[%d]: must not be empty", index)
			continue
		}
		if !nameRE.MatchString(dep) {
			add("StartConditions.DependsOn[%d] %q: only lowercase [a-z0-9._-] allowed, must start alphanumeric", index, dep)
		}
	}
}

// checkNetworkCapabilities forbids network-administration capabilities on a filtered
// app. A filtered app (one with NetworkLists) runs inside the pod whose network
// namespace carries the nftables egress lock-down; granting CAP_NET_ADMIN (or the
// superset CAP_SYS_ADMIN) would let the app flush that ruleset at runtime and reach an
// unfiltered network. Enforcement and app-granted netns control are mutually exclusive
// by design (section 5.3).
func checkNetworkCapabilities(cfg schema.AppConfig, add addFunc) {
	if len(cfg.NetworkMeta.NetworkLists) == 0 {
		return // unfiltered app runs with --network none; NET_ADMIN reaches nothing
	}
	for index, capability := range cfg.Capabilities {
		switch strings.TrimPrefix(strings.ToUpper(capability), "CAP_") {
		case "NET_ADMIN", "SYS_ADMIN":
			add("Capabilities[%d] %q: cannot be granted to an app with NetworkLists - it could flush the egress lock-down in the pod netns and escape the network filter (section 5.3)", index, capability)
		}
	}
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

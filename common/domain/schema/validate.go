package schema

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// nameRE is the charset for an AppNameID / container name and for a target
// ContainerName: lowercase [a-z0-9._-], starting alphanumeric. These become podman
// object names / CLI arguments, so the charset keeps them safe to pass verbatim.
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// digestRE matches a canonical sha256 digest pin at the end of an image reference:
// "@sha256:" then exactly 64 lowercase hex chars. A merely-contained "@sha256:" let
// a fake/short digest pass while smuggling extra Containerfile directives into the
// derived FROM line (§5.5), so the pin must be the canonical, anchored form.
var digestRE = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// ifaceRE is the strict charset for a NetworkList Interface: a comma or space would
// splice extra pasta options into `--network pasta:--interface,<iface>`, so only an
// interface-name charset (no commas/spaces) is allowed.
var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// capRE is the charset for one Capabilities entry: an optional CAP_ prefix then
// uppercase letters/underscores. It rejects empty, lowercase, a leading '-', and
// option-smuggling; the ALL grant is rejected separately (it is uppercase-clean).
var capRE = regexp.MustCompile(`^(CAP_)?[A-Z_]+$`)

// hasUnsafe reports whether str contains whitespace or any control character — the
// metacharacters that, smuggled into an image ref, a path, or a colon-delimited
// podman field, would inject extra directives/flags or shift the field layout.
func hasUnsafe(str string) bool {
	for _, run := range str {
		if run == ' ' || run == '\t' || run == '\r' || run == '\n' || run < 0x20 || run == 0x7f {
			return true
		}
	}
	return false
}

// Validate checks an AppConfig against the hard schema rules. It is a pure function
// (no I/O) so it runs identically at save time (zcc) and launch time (zcr). All
// problems are returned joined together, not just the first. Non-fatal advisories
// live in Warnings, not here — everything Validate reports blocks the config.
func Validate(cfg AppConfig) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if cfg.SchemaVersion != SchemaVersion {
		add("SchemaVersion: got %d, want %d", cfg.SchemaVersion, SchemaVersion)
	}

	switch cfg.Type {
	case ZincContainer:
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
		// The image is interpolated into a derived Containerfile FROM line; a
		// space/tab/CR/LF would smuggle extra directives, so it must be a single-line
		// reference (§5.5).
		add("ImageMeta.Image %q: must be a single-line reference (no whitespace or control characters)", cfg.ImageMeta.Image)
	case !TrustedImage(cfg.ImageMeta.Image) && !digestRE.MatchString(cfg.ImageMeta.Image):
		// §5.5: third-party images are pinned by a *canonical* digest (@sha256: + 64
		// hex); only locally built trusted-* images may use a mutable local tag.
		add("ImageMeta.Image %q: third-party images must be digest-pinned (…@sha256:<64 hex>); only trusted-* images may use a local tag (§5.5)", cfg.ImageMeta.Image)
	}

	// Terminal / multiterminal / background interplay (§9.1).
	start := cfg.StartConditions
	switch {
	case start.Multiterminal && !start.Terminal:
		add("StartConditions.Multiterminal: requires Terminal (it spawns terminals into a shared container)")
	case start.Terminal && cfg.StopConditions.Background && !start.Multiterminal:
		// A plain terminal app runs interactively in a spawned window; detaching it to
		// the background is contradictory. Multiterminal lifts this: there Background
		// means "keep the shared container alive after the last terminal closes".
		add("StartConditions.Terminal: a terminal app runs in a foreground window; it cannot also be StopConditions.Background (use Multiterminal to keep the shared container alive after the last terminal closes)")
	}
	if start.Multiterminal && strings.TrimSpace(start.Entrypoint) == "" && strings.TrimSpace(start.MultiterminalEntrypoint) == "" {
		// Each terminal re-runs the app via `podman exec`, which needs an explicit
		// command — a holder occupies PID 1, so the image's default cannot be replayed.
		add("StartConditions: Multiterminal needs an explicit Entrypoint or MultiterminalEntrypoint (the image default cannot be replayed into each terminal)")
	}

	// NetworkLists — format validation only (Layer A). List order is priority (first
	// wins). whitelist + empty allowlist = deny-all (valid, fail-closed); an empty
	// blacklist (allow-all) is a create-time Warning, not an error.
	for index, netList := range cfg.NetworkMeta.NetworkLists {
		for _, cidr := range netList.IPv4CIDR {
			if !validCIDR(cidr, false) {
				add("NetworkLists[%d].IPv4CIDR %q: not a valid IPv4 CIDR", index, cidr)
			}
		}
		for _, cidr := range netList.IPv6CIDR {
			if !validCIDR(cidr, true) {
				add("NetworkLists[%d].IPv6CIDR %q: not a valid IPv6 CIDR", index, cidr)
			}
		}
		for _, port := range netList.Ports {
			if port < 1 || port > 65535 {
				add("NetworkLists[%d].Ports %d: out of range 1-65535", index, port)
			}
		}
		if iface := netList.Interface; iface != "" && !ifaceRE.MatchString(iface) {
			add("NetworkLists[%d].Interface %q: only [A-Za-z0-9._-] allowed (no commas or spaces)", index, iface)
		}
		if !netList.Host {
			// A container-scoped list names the container it filters; the name is
			// checkable now, existence is verified at launch (§6.6).
			switch {
			case strings.TrimSpace(netList.ContainerName) == "":
				add("NetworkLists[%d].ContainerName: required when Host=false", index)
			case !nameRE.MatchString(netList.ContainerName):
				add("NetworkLists[%d].ContainerName %q: invalid container name; allowed [a-z0-9._-], must start alphanumeric", index, netList.ContainerName)
			}
		}
	}

	// Volumes are host-side mounts; Configs share the Volume type but source from the
	// app bundle (apps/<name>/configs/…) and must not escape it. host:container is
	// colon-delimited in `podman -v`, so a ':'/','/whitespace in any path shifts
	// podman's fields (e.g. claim "ro" but mount "rw") — rejected everywhere below.
	for index, volume := range cfg.Volumes {
		checkInner("Volumes", index, volume.InnerMount, add)
		if volume.HostMounted {
			switch {
			case strings.TrimSpace(volume.HostMount) == "":
				add("Volumes[%d].HostMount: required when HostMounted=true", index)
			case hasUnsafe(volume.HostMount) || strings.ContainsAny(volume.HostMount, ":,"):
				add("Volumes[%d].HostMount %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", index, volume.HostMount)
			}
		}
		checkSizeLimit("Volumes", index, volume, add)
	}
	for index, configMount := range cfg.Configs {
		checkInner("Configs", index, configMount.InnerMount, add)
		// Bundle-relative source: reject an absolute path, a leading '/', and any '..'
		// segment so a "Config" can't quietly mount an arbitrary host path (HostMounted
		// is ignored here — a Config is always bundle-sourced).
		source := configMount.HostMount
		switch {
		case strings.TrimSpace(source) == "":
			add("Configs[%d].HostMount: must name a path under the app bundle (apps/<name>/configs/)", index)
		case hasUnsafe(source) || strings.ContainsAny(source, ":,"):
			add("Configs[%d].HostMount %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", index, source)
		case strings.HasPrefix(source, "/"):
			add("Configs[%d].HostMount %q: must be relative to the app bundle, not an absolute path", index, source)
		case hasDotDot(source):
			add("Configs[%d].HostMount %q: must not escape the app bundle (no '..' segments)", index, source)
		}
		checkSizeLimit("Configs", index, configMount, add)
	}

	// Keys are mounted as `path:dest:ro` (colon-delimited), same field-shift risk.
	for index, keyEntry := range cfg.Keys {
		switch keyEntry.Type {
		case SSH, GPG:
		case "":
			add("Keys[%d].Type: must be set (SSH|GPG)", index)
		default:
			add("Keys[%d].Type %q: must be SSH|GPG", index, keyEntry.Type)
		}
		switch {
		case strings.TrimSpace(keyEntry.Path) == "":
			add("Keys[%d].Path: must not be empty", index)
		case hasUnsafe(keyEntry.Path) || strings.ContainsAny(keyEntry.Path, ":,"):
			add("Keys[%d].Path %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", index, keyEntry.Path)
		}
	}

	// Capabilities are passed verbatim to `--cap-add`; without a guard a config could
	// add ALL/SYS_ADMIN or smuggle options. Reject the ALL grant outright and require
	// a strict capability-name charset.
	for index, capability := range cfg.Capabilities {
		bare := strings.TrimPrefix(strings.ToUpper(capability), "CAP_")
		switch {
		case bare == "ALL":
			add("Capabilities[%d] %q: granting ALL capabilities is forbidden (add only the specific caps an app needs)", index, capability)
		case !capRE.MatchString(capability):
			add("Capabilities[%d] %q: only an (optional) CAP_ prefix then [A-Z_] allowed (e.g. NET_ADMIN or CAP_NET_ADMIN)", index, capability)
		}
	}

	// Resource caps: 0 means "unlimited" for CPU/RAM/PIDs, so only negatives are
	// invalid.
	res := cfg.ResourcesMeta
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

	// A non-root user name becomes `podman --user`; keep it a safe charset.
	if name := cfg.InternalUserMeta.NonRootUserName; name != "" && !nameRE.MatchString(name) {
		add("InternalUserMeta.NonRootUserName %q: only lowercase [a-z0-9._-] allowed, must start alphanumeric", name)
	}

	return errors.Join(errs...)
}

// Warnings returns non-fatal advisories surfaced at create time (zcc). Unlike
// Validate, nothing here blocks a save or a launch — it only flags configs that are
// valid but probably not what the author intended.
func Warnings(cfg AppConfig) []string {
	var warns []string
	for index, netList := range cfg.NetworkMeta.NetworkLists {
		// A blacklist with no entries blocks nothing — i.e. allow-all. Legal, but on a
		// security-first tool it is worth surfacing so it isn't a silent open door.
		if netList.Blacklist &&
			len(netList.IPv4CIDR) == 0 && len(netList.IPv6CIDR) == 0 && len(netList.Ports) == 0 {
			warns = append(warns, fmt.Sprintf(
				"NetworkLists[%d]: blacklist with no CIDRs/ports blocks nothing (allow-all)", index))
		}
	}
	return warns
}

// checkInner validates a container-side mount path (InnerMount) for a Volume or a
// Config: non-empty and free of the ':'/','/whitespace that shifts podman's -v fields.
func checkInner(list string, index int, inner string, add func(string, ...any)) {
	switch {
	case strings.TrimSpace(inner) == "":
		add("%s[%d].InnerMount: must not be empty", list, index)
	case hasUnsafe(inner) || strings.ContainsAny(inner, ":,"):
		add("%s[%d].InnerMount %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", list, index, inner)
	}
}

// checkSizeLimit rejects the no-op combination SizeLimited=true with a non-positive
// SizeLimitMiB, so an explicit-but-empty cap can't sit in a config reading as applied.
func checkSizeLimit(list string, index int, volume Volume, add func(string, ...any)) {
	if volume.SizeLimited && volume.SizeLimitMiB <= 0 {
		add("%s[%d]: SizeLimited is set but SizeLimitMiB is %d (a size limit must be > 0, or clear SizeLimited)", list, index, volume.SizeLimitMiB)
	}
}

// hasDotDot reports whether a slash-separated relative path contains a ".." segment
// (a bundle escape). Paths here are container/bundle paths, so "/" is the separator.
func hasDotDot(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// validCIDR reports whether cidr is a valid CIDR in the requested family (wantV6 =
// true for IPv6, false for IPv4). The family check stops an address being configured
// under the wrong key, e.g. an IPv6 range in IPv4CIDR.
func validCIDR(cidr string, wantV6 bool) bool {
	addr, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return (addr.To4() == nil) == wantV6
}

// TrustedImage reports whether image refers to a locally built trusted-* image
// (§5.5/§7), e.g. "trusted-go-dev", "trusted-go-dev:latest", or
// "localhost/trusted-go-dev:latest". Trusted images may use a mutable local tag;
// every other (third-party) image must be pinned by digest.
func TrustedImage(image string) bool {
	ref := image
	if atIdx := strings.IndexByte(ref, '@'); atIdx >= 0 { // drop @sha256:… digest
		ref = ref[:atIdx]
	}
	if slashIdx := strings.LastIndexByte(ref, '/'); slashIdx >= 0 { // drop registry/namespace
		ref = ref[slashIdx+1:]
	}
	if colonIdx := strings.IndexByte(ref, ':'); colonIdx >= 0 { // drop :tag
		ref = ref[:colonIdx]
	}
	return strings.HasPrefix(ref, "trusted-")
}

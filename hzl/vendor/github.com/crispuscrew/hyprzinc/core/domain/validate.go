package domain

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// digestRE matches a canonical sha256 digest pin at the end of an image reference:
// "@sha256:" then exactly 64 lowercase hex chars. A merely-contained "@sha256:" let
// a fake/short digest pass while smuggling extra Containerfile directives into the
// derived FROM line (§5.5), so the pin must be the canonical, anchored form.
var digestRE = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// ifaceRE is the strict charset for network.interface: a comma or space would splice
// extra pasta options into `--network pasta:--interface,<iface>`, so only an
// interface-name charset (no commas/spaces) is allowed.
var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// capRE is the charset for one capabilities.extra entry: an optional CAP_ prefix then
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

// Validate checks an AppConfig against the schema rules in docs §3–§5. It is a
// pure function (no I/O) so it can run identically at save time and launch time.
// All problems are returned joined together, not just the first.
func Validate(cfg AppConfig) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if cfg.SchemaVersion != SchemaVersion {
		add("schema_version: got %d, want %d", cfg.SchemaVersion, SchemaVersion)
	}

	switch {
	case strings.TrimSpace(cfg.App.Name) == "":
		add("app.name: must not be empty")
	case !nameRE.MatchString(cfg.App.Name):
		add("app.name %q: only lowercase [a-z0-9._-] allowed, must start alphanumeric", cfg.App.Name)
	}
	switch {
	case strings.TrimSpace(cfg.App.Image) == "":
		add("app.image: must not be empty")
	case hasUnsafe(cfg.App.Image):
		// The image is interpolated into a derived Containerfile FROM line (derived.go);
		// a space/tab/CR/LF would smuggle extra directives, so it must be a single-line
		// reference (§5.5).
		add("app.image %q: must be a single-line reference (no whitespace or control characters)", cfg.App.Image)
	case !TrustedImage(cfg.App.Image) && !digestRE.MatchString(cfg.App.Image):
		// §5.5: third-party images are pinned by a *canonical* digest (@sha256: + 64
		// hex); only locally built trusted-* images may use a mutable local tag. A mere
		// substring match let a fake/short digest pass the digest-pin audit.
		add("app.image %q: third-party images must be digest-pinned (…@sha256:<64 hex>); only trusted-* images may use a local tag (§5.5)", cfg.App.Image)
	}
	if cfg.App.Preset != "" && !ValidPreset(cfg.App.Preset) {
		add("app.preset %q: must be strict|standard|networked", cfg.App.Preset)
	}
	if cfg.App.AutostartWorkspace < 0 {
		add("app.autostart_workspace: must be >= 0 (0 = no preference)")
	}
	// Terminal / multiterminal / background interplay (§9.1).
	switch {
	case cfg.App.Multiterminal && !cfg.App.Terminal:
		add("app.multiterminal: requires app.terminal (it spawns terminals into a shared container)")
	case cfg.App.Terminal && cfg.App.Background && !cfg.App.Multiterminal:
		// A plain terminal app runs interactively in a spawned window; detaching it
		// to the background is contradictory. Multiterminal lifts this: there
		// background means "keep the shared container alive after the last terminal".
		add("app.terminal: a terminal app runs in a foreground terminal window; it cannot also be background (set multiterminal to keep the shared container alive after the last terminal closes)")
	}
	if cfg.App.Multiterminal && len(cfg.App.Command) == 0 {
		// Each terminal re-runs the app via `podman exec`, which needs an explicit
		// argv — a holder occupies PID 1, so the image's ENTRYPOINT/CMD never runs
		// and cannot be replayed.
		add("app.command: multiterminal needs an explicit command to run in each terminal (the image's default command cannot be replayed)")
	}

	switch cfg.Display.Wayland {
	case WaylandSecurityContext, WaylandPassthrough:
	case "":
		add("display.wayland: must be set (security-context|passthrough)")
	default:
		add("display.wayland %q: must be security-context|passthrough", cfg.Display.Wayland)
	}

	// Fields that belong to one mode are rejected under the others, so a stale
	// allowlist left over after a mode switch can't silently read as "applied".
	denyPastaFields := func() {
		mode := cfg.Network.Mode
		if len(cfg.Network.IPv4CIDR) > 0 {
			add("network.ipv4_cidr: only valid when network.mode = %q (mode is %q)", NetworkPasta, mode)
		}
		if len(cfg.Network.IPv6CIDR) > 0 {
			add("network.ipv6_cidr: only valid when network.mode = %q (mode is %q)", NetworkPasta, mode)
		}
		if len(cfg.Network.Ports) > 0 {
			add("network.ports: only valid when network.mode = %q (mode is %q)", NetworkPasta, mode)
		}
		if strings.TrimSpace(cfg.Network.Interface) != "" {
			add("network.interface: only valid when network.mode = %q (mode is %q)", NetworkPasta, mode)
		}
		if cfg.Network.BlockDNS {
			add("network.block_dns: only valid when network.mode = %q (mode is %q)", NetworkPasta, mode)
		}
	}
	denyTarget := func() {
		if strings.TrimSpace(cfg.Network.Target) != "" {
			add("network.target: only valid when network.mode = %q (mode is %q)", NetworkContainer, cfg.Network.Mode)
		}
	}

	switch cfg.Network.Mode {
	case NetworkNone:
		denyPastaFields()
		denyTarget()
	case NetworkPasta:
		for _, cidr := range cfg.Network.IPv4CIDR {
			if !validCIDR(cidr, false) {
				add("network.ipv4_cidr %q: not a valid IPv4 CIDR", cidr)
			}
		}
		for _, cidr := range cfg.Network.IPv6CIDR {
			if !validCIDR(cidr, true) {
				add("network.ipv6_cidr %q: not a valid IPv6 CIDR", cidr)
			}
		}
		for _, port := range cfg.Network.Ports {
			if port < 1 || port > 65535 {
				add("network.ports %d: out of range 1-65535", port)
			}
		}
		// The interface is spliced into `--network pasta:--interface,<iface>`; a comma
		// or space would inject extra pasta options, so require a strict interface-name
		// charset (netenforce/pasta.go).
		if iface := cfg.Network.Interface; iface != "" && !ifaceRE.MatchString(iface) {
			add("network.interface %q: only [A-Za-z0-9._-] allowed (no commas or spaces)", iface)
		}
		denyTarget()
	case NetworkContainer:
		// Format is checkable now; existence is not — the target may not exist yet
		// (depends_on auto-starts it), so that check belongs at launch time (§6.6).
		switch {
		case strings.TrimSpace(cfg.Network.Target) == "":
			add(`network.target: required when network.mode = "container"`)
		case !nameRE.MatchString(cfg.Network.Target):
			add("network.target %q: invalid container name; allowed [a-z0-9._-], must start alphanumeric (existence verified at launch)", cfg.Network.Target)
		}
		denyPastaFields()
	case "":
		add("network.mode: must be set (none|pasta|container)")
	default:
		add("network.mode %q: must be none|pasta|container", cfg.Network.Mode)
	}

	for index, mount := range cfg.Mounts {
		// host:container:mode is colon-delimited in `podman -v`; a ':', comma, or
		// whitespace/control char inside host or container shifts podman's fields (e.g.
		// claim "ro" but actually mount "rw"), so reject those metacharacters outright.
		switch {
		case strings.TrimSpace(mount.Host) == "":
			add("mounts[%d].host: must not be empty", index)
		case hasUnsafe(mount.Host) || strings.ContainsAny(mount.Host, ":,"):
			add("mounts[%d].host %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", index, mount.Host)
		}
		switch {
		case strings.TrimSpace(mount.Container) == "":
			add("mounts[%d].container: must not be empty", index)
		case hasUnsafe(mount.Container) || strings.ContainsAny(mount.Container, ":,"):
			add("mounts[%d].container %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", index, mount.Container)
		}
		switch mount.Mode {
		case MountRO, MountRW:
		case "":
			add("mounts[%d].mode: must be set (ro|rw)", index)
		default:
			add("mounts[%d].mode %q: must be ro|rw", index, mount.Mode)
		}
	}

	// Keys are mounted as `path:dest:ro` (colon-delimited), same field-shift risk as
	// mounts above, so apply the same metacharacter rejection to every key path.
	checkKeyPath := func(field string, index int, path string) {
		switch {
		case strings.TrimSpace(path) == "":
			add("keys.%s[%d]: must not be empty", field, index)
		case hasUnsafe(path) || strings.ContainsAny(path, ":,"):
			add("keys.%s[%d] %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", field, index, path)
		}
	}
	for index, path := range cfg.Keys.SSH {
		checkKeyPath("ssh", index, path)
	}
	for index, path := range cfg.Keys.GPG {
		checkKeyPath("gpg", index, path)
	}

	// Extra capabilities are passed verbatim to `--cap-add` (runtime.go); without a
	// guard a config could add ALL/SYS_ADMIN or smuggle options. Reject the ALL grant
	// outright and require a strict capability-name charset.
	for index, capability := range cfg.Capabilities.Extra {
		bare := strings.TrimPrefix(strings.ToUpper(capability), "CAP_")
		switch {
		case bare == "ALL":
			add("capabilities.extra[%d] %q: granting ALL capabilities is forbidden (re-add only the specific caps an app needs)", index, capability)
		case !capRE.MatchString(capability):
			add("capabilities.extra[%d] %q: only an (optional) CAP_ prefix then [A-Z_] allowed (e.g. NET_ADMIN or CAP_NET_ADMIN)", index, capability)
		}
	}

	switch cfg.Theme.Mode {
	case ThemeHost, ThemeNone:
	case "":
		add("theme.mode: must be set (host|none)")
	default:
		add("theme.mode %q: must be host|none", cfg.Theme.Mode)
	}

	return errors.Join(errs...)
}

// validCIDR reports whether cidr is a valid CIDR in the requested family (wantV6 =
// true for IPv6, false for IPv4). The family check stops an address being
// configured under the wrong key, e.g. an IPv6 range in network.ipv4_cidr.
func validCIDR(cidr string, wantV6 bool) bool {
	addr, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return (addr.To4() == nil) == wantV6
}

// TrustedImage reports whether image refers to a locally built trusted-* image
// (docs §5.5/§7), e.g. "trusted-go-dev", "trusted-go-dev:latest", or
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

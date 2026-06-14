package config

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

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
	if strings.TrimSpace(cfg.App.Image) == "" {
		add("app.image: must not be empty")
	} else if !TrustedImage(cfg.App.Image) && !strings.Contains(cfg.App.Image, "@sha256:") {
		// §5.5: third-party images are pinned by digest; only locally built
		// trusted-* images may be referenced by a mutable local tag.
		add("app.image %q: third-party images must be digest-pinned (…@sha256:…); only trusted-* images may use a local tag (§5.5)", cfg.App.Image)
	}
	if cfg.App.Preset != "" && !ValidPreset(cfg.App.Preset) {
		add("app.preset %q: must be strict|standard|networked", cfg.App.Preset)
	}
	if cfg.App.AutostartWorkspace < 0 {
		add("app.autostart_workspace: must be >= 0 (0 = no preference)")
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
		if strings.TrimSpace(mount.Host) == "" {
			add("mounts[%d].host: must not be empty", index)
		}
		if strings.TrimSpace(mount.Container) == "" {
			add("mounts[%d].container: must not be empty", index)
		}
		switch mount.Mode {
		case MountRO, MountRW:
		case "":
			add("mounts[%d].mode: must be set (ro|rw)", index)
		default:
			add("mounts[%d].mode %q: must be ro|rw", index, mount.Mode)
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
	if at := strings.IndexByte(ref, '@'); at >= 0 { // drop @sha256:… digest
		ref = ref[:at]
	}
	if slash := strings.LastIndexByte(ref, '/'); slash >= 0 { // drop registry/namespace
		ref = ref[slash+1:]
	}
	if colon := strings.IndexByte(ref, ':'); colon >= 0 { // drop :tag
		ref = ref[:colon]
	}
	return strings.HasPrefix(ref, "trusted-")
}

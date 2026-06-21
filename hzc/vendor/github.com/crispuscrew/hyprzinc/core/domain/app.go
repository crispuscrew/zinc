// Package domain is HyprZinc's pure core: the on-disk app-config schema, the
// validation rules, the preset templates, and the derived-image policy. It is the
// hexagon's center — no filesystem, no podman, no environment. Everything here is a
// pure function over data, so it runs identically at save time and launch time and
// is trivially testable (docs/architecture.md §3, §13).
//
// I/O lives in the adapters (core/adapters/*); orchestration in core/app; the
// contracts between them in core/ports. domain depends on none of those.
package domain

// SchemaVersion is the only app-config schema version this build understands.
const SchemaVersion = 1

// Enumerated field values (docs §3–§5).
const (
	WaylandSecurityContext = "security-context"
	WaylandPassthrough     = "passthrough"

	NetworkNone      = "none"
	NetworkPasta     = "pasta"
	NetworkContainer = "container"

	MountRO = "ro"
	MountRW = "rw"

	ThemeHost = "host"
	ThemeNone = "none"
)

// AppConfig is one app definition: ~/.config/hyprzinc/apps/<name>.toml (§3).
type AppConfig struct {
	SchemaVersion int          `toml:"schema_version"`
	App           App          `toml:"app"`
	Display       Display      `toml:"display"`
	Network       Network      `toml:"network"`
	Mounts        []Mount      `toml:"mounts"`
	Keys          Keys         `toml:"keys"`
	Audio         Audio        `toml:"audio"`
	Theme         Theme        `toml:"theme"`
	Capabilities  Capabilities `toml:"capabilities"`
	DependsOn     DependsOn    `toml:"depends_on"`
}

type App struct {
	Name               string   `toml:"name"`
	Image              string   `toml:"image"`   // digest-pinned (third-party) or local tag (trusted-*); §5.5
	Command            []string `toml:"command"` // argv appended after the image — overrides the image's default command (CMD); empty = image default
	Install            string   `toml:"install"` // build-time setup run as a RUN layer atop image to make a derived image (FROM image + RUN install); empty = run image directly (§5.5)
	Preset             string   `toml:"preset"`
	Description        string   `toml:"description"`
	Icon               string   `toml:"icon"`
	Terminal           bool     `toml:"terminal"`      // CLI/TUI app: launch inside a terminal emulator window (§9.1, §11)
	Multiterminal      bool     `toml:"multiterminal"` // terminal app: a long-lived holder so many terminals can attach to one instance; container lives until the last closes (§9.1)
	Background         bool     `toml:"background"`
	Autostart          bool     `toml:"autostart"`
	AutostartWorkspace int      `toml:"autostart_workspace"`
}

type Display struct {
	Wayland string `toml:"wayland"` // security-context | passthrough
	GPU     bool   `toml:"gpu"`
}

type Network struct {
	Mode string `toml:"mode"` // none | pasta | container

	// mode = "pasta" — egress allowlist (enforced by nftables in the netns, §5.3)
	IPv4CIDR  []string `toml:"ipv4_cidr"`
	IPv6CIDR  []string `toml:"ipv6_cidr"`
	Ports     []int    `toml:"ports"`
	Interface string   `toml:"interface"`
	BlockDNS  bool     `toml:"block_dns"`

	// mode = "container"
	Target string `toml:"target"`
}

type Mount struct {
	Host      string `toml:"host"`
	Container string `toml:"container"`
	Mode      string `toml:"mode"` // ro | rw
}

// Keys is a convenience layer for SSH/GPG only (§3 [keys]): unlike [[mounts]] it
// also wires the agent socket and enforces 0600 inside the container.
type Keys struct {
	SSH []string `toml:"ssh"`
	GPG []string `toml:"gpg"`
}

type Audio struct {
	Pipewire   bool `toml:"pipewire"`
	LegacyALSA bool `toml:"legacy_alsa"`
}

type Theme struct {
	Mode string `toml:"mode"` // host | none
}

type Capabilities struct {
	Extra []string `toml:"extra"` // podman --cap-add entries
}

type DependsOn struct {
	Containers []string `toml:"containers"`
}

// Package config defines HyprZinc's on-disk configuration schema and the pure
// functions that parse and validate it.
//
// It is the functional core for app definitions: Load reads a file, Validate is
// a pure function over the decoded data (no I/O). Because validation is pure it
// runs at both save time and launch time — the launch-time check is what catches
// hand edits and drift (docs/architecture.md §3). hzp is the sole writer of these
// files; Nix only seeds them on first activation (§9.3).
package config

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
	Name               string `toml:"name"`
	Image              string `toml:"image"` // digest-pinned (third-party) or local tag (trusted-*); §5.5
	Preset             string `toml:"preset"`
	Description        string `toml:"description"`
	Icon               string `toml:"icon"`
	Background         bool   `toml:"background"`
	Autostart          bool   `toml:"autostart"`
	AutostartWorkspace int    `toml:"autostart_workspace"`
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

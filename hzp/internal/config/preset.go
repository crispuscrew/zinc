package config

// Preset names (docs §4). Presets are starting *templates* only — not enforced
// modes. Every field is independently overridable, and hzp shows each field's
// actual value alongside the preset label so the user sees the truth.
const (
	PresetStrict     = "strict"
	PresetStandard   = "standard"
	PresetNetworked  = "networked"
	PresetDefaultNew = PresetStrict // default for a newly created app (§4)
)

// DefaultsFor returns the field values a preset seeds. ok is false for an unknown
// preset name. The result is a template to start from, not a constraint.
//
//	field             strict             standard      networked
//	network.mode      none               none          pasta
//	network.block_dns —                  —             true
//	display.wayland   security-context   passthrough   passthrough
//	display.gpu       false              false         false
//	audio.pipewire    false              false         false
//	theme.mode        host               host          host
func DefaultsFor(preset string) (AppConfig, bool) {
	base := AppConfig{
		SchemaVersion: SchemaVersion,
		App:           App{Preset: preset},
		Display:       Display{Wayland: WaylandSecurityContext, GPU: false},
		Network:       Network{Mode: NetworkNone},
		Audio:         Audio{Pipewire: false, LegacyALSA: false},
		Theme:         Theme{Mode: ThemeHost},
	}
	switch preset {
	case PresetStrict:
		// base is already strict
	case PresetStandard:
		base.Display.Wayland = WaylandPassthrough
	case PresetNetworked:
		base.Display.Wayland = WaylandPassthrough
		base.Network.Mode = NetworkPasta
		base.Network.BlockDNS = true // DNS-leak guard is meaningful only with egress (§5.3)
	default:
		return AppConfig{}, false
	}
	return base, true
}

// ValidPreset reports whether name is a known preset.
func ValidPreset(name string) bool {
	_, valid := DefaultsFor(name)
	return valid
}

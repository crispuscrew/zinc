package domain

import "testing"

// digestPin is a canonical sha256 digest (64 hex chars) — the pinned form §5.5 now
// requires for third-party images.
const digestPin = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validApp() AppConfig {
	return AppConfig{
		SchemaVersion: SchemaVersion,
		App:           App{Name: "firefox", Image: "docker.io/library/firefox" + digestPin, Preset: PresetStrict},
		Display:       Display{Wayland: WaylandSecurityContext},
		Network:       Network{Mode: NetworkNone},
		Theme:         Theme{Mode: ThemeHost},
	}
}

func TestValidate_OK(t *testing.T) {
	if err := Validate(validApp()); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := map[string]func(*AppConfig){
		"schema":           func(cfg *AppConfig) { cfg.SchemaVersion = 2 },
		"empty name":       func(cfg *AppConfig) { cfg.App.Name = "" },
		"bad name":         func(cfg *AppConfig) { cfg.App.Name = "Fire Fox" },
		"empty image":      func(cfg *AppConfig) { cfg.App.Image = "" },
		"bad preset":       func(cfg *AppConfig) { cfg.App.Preset = "paranoid" },
		"bad wayland":      func(cfg *AppConfig) { cfg.Display.Wayland = "x11" },
		"bad netmode":      func(cfg *AppConfig) { cfg.Network.Mode = "bridge" },
		"container no tgt": func(cfg *AppConfig) { cfg.Network.Mode = NetworkContainer; cfg.Network.Target = "" },
		"bad target name":  func(cfg *AppConfig) { cfg.Network.Mode = NetworkContainer; cfg.Network.Target = "Bad Name" },
		"bad cidr":         func(cfg *AppConfig) { cfg.Network.Mode = NetworkPasta; cfg.Network.IPv4CIDR = []string{"oops"} },
		"cidr wrong family": func(cfg *AppConfig) {
			cfg.Network.Mode = NetworkPasta
			cfg.Network.IPv4CIDR = []string{"2001:db8::/32"}
		},
		"bad port":            func(cfg *AppConfig) { cfg.Network.Mode = NetworkPasta; cfg.Network.Ports = []int{70000} },
		"irrelevant cidr":     func(cfg *AppConfig) { cfg.Network.IPv4CIDR = []string{"1.1.1.1/32"} },
		"irrelevant blockdns": func(cfg *AppConfig) { cfg.Network.BlockDNS = true },
		"irrelevant target":   func(cfg *AppConfig) { cfg.Network.Mode = NetworkPasta; cfg.Network.Target = "vpn-container" },
		"bad mount mode":      func(cfg *AppConfig) { cfg.Mounts = []Mount{{Host: "/a", Container: "/b", Mode: "x"}} },
		"mount no host":       func(cfg *AppConfig) { cfg.Mounts = []Mount{{Container: "/b", Mode: MountRO}} },
		"bad theme":           func(cfg *AppConfig) { cfg.Theme.Mode = "dark" },
		// Field-shift / injection metacharacters in colon-delimited podman fields.
		"mount host colon":      func(cfg *AppConfig) { cfg.Mounts = []Mount{{Host: "/a:rw", Container: "/b", Mode: MountRO}} },
		"mount container colon": func(cfg *AppConfig) { cfg.Mounts = []Mount{{Host: "/a", Container: "/b:extra", Mode: MountRO}} },
		"ssh key colon":         func(cfg *AppConfig) { cfg.Keys.SSH = []string{"/k/id:rsa"} },
		"gpg key colon":         func(cfg *AppConfig) { cfg.Keys.GPG = []string{"/k/key:bad"} },
		// pasta interface option-smuggling via a comma.
		"iface comma": func(cfg *AppConfig) {
			cfg.Network.Mode = NetworkPasta
			cfg.Network.Interface = "eth0,--map-gw"
		},
		// capabilities.extra: the ALL grant and option-smuggling are forbidden.
		"cap all":             func(cfg *AppConfig) { cfg.Capabilities.Extra = []string{"ALL"} },
		"cap all cap":         func(cfg *AppConfig) { cfg.Capabilities.Extra = []string{"CAP_ALL"} },
		"cap bad":             func(cfg *AppConfig) { cfg.Capabilities.Extra = []string{"net_admin --privileged"} },
		"terminal+background": func(cfg *AppConfig) { cfg.App.Terminal = true; cfg.App.Background = true },
		"multiterm no terminal": func(cfg *AppConfig) {
			cfg.App.Multiterminal = true
			cfg.App.Command = []string{"htop"}
		},
		"multiterm no command": func(cfg *AppConfig) {
			cfg.App.Terminal = true
			cfg.App.Multiterminal = true
		},
		// keep_open holds a terminal window open after exit; it is meaningless without
		// a terminal, so it must be rejected rather than silently ignored.
		"keep_open no terminal": func(cfg *AppConfig) { cfg.App.KeepOpen = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := validApp()
			mutate(&cfg)
			if err := Validate(cfg); err == nil {
				t.Fatalf("expected error for %q, got nil", name)
			}
		})
	}
}

func TestValidate_Multiterminal_OK(t *testing.T) {
	// multiterminal needs terminal + an explicit command; background is allowed
	// (it means "keep the shared container after the last terminal closes").
	for _, bg := range []bool{false, true} {
		cfg := validApp()
		cfg.App.Terminal = true
		cfg.App.Multiterminal = true
		cfg.App.Background = bg
		cfg.App.Command = []string{"htop"}
		if err := Validate(cfg); err != nil {
			t.Fatalf("multiterminal (background=%v) should validate, got: %v", bg, err)
		}
	}
}

func TestValidate_KeepOpen_OK(t *testing.T) {
	cfg := validApp()
	cfg.App.Terminal = true
	cfg.App.KeepOpen = true
	if err := Validate(cfg); err != nil {
		t.Fatalf("keep_open on a terminal app should validate, got: %v", err)
	}
}

func TestValidate_ImagePolicy(t *testing.T) {
	// Third-party images must be digest-pinned; trusted-* may use a local tag (§5.5).
	cases := map[string]struct {
		image string
		valid bool
	}{
		"third-party digest":  {"docker.io/library/firefox" + digestPin, true},
		"third-party tag":     {"docker.io/library/firefox:latest", false},
		"third-party bare":    {"alpine", false},
		"short digest":        {"docker.io/library/firefox@sha256:abc", false}, // not 64 hex
		"image with newline":  {"alpine" + digestPin + "\nRUN echo pwned", false},
		"trusted tag":         {"trusted-go-dev:latest", true},
		"trusted bare":        {"trusted-base", true},
		"trusted with host":   {"localhost/trusted-rust:latest", true},
		"trusted with digest": {"trusted-go" + digestPin, true},
		"not-quite-trusted":   {"trustedish:latest", false},
	}
	for name, tcase := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := validApp()
			cfg.App.Image = tcase.image
			err := Validate(cfg)
			if tcase.valid && err != nil {
				t.Fatalf("image %q should validate, got: %v", tcase.image, err)
			}
			if !tcase.valid && err == nil {
				t.Fatalf("image %q should be rejected, got nil", tcase.image)
			}
		})
	}
}

func TestValidate_ContainerMode_OK(t *testing.T) {
	cfg := validApp()
	cfg.Network = Network{Mode: NetworkContainer, Target: "vpn-container"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("container mode with target should be valid: %v", err)
	}
}

func TestValidate_PastaAllowlist_OK(t *testing.T) {
	cfg := validApp()
	cfg.Network = Network{
		Mode:     NetworkPasta,
		IPv4CIDR: []string{"1.1.1.1/32"},
		IPv6CIDR: []string{"2001:db8::/32"},
		Ports:    []int{443, 80},
		BlockDNS: true,
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("valid pasta allowlist should pass: %v", err)
	}
}

func TestValidate_NewRules_OK(t *testing.T) {
	// Each new rule's benign form must still validate: a plain interface name, a
	// specific capability (bare and CAP_-prefixed), and metacharacter-free paths.
	pasta := validApp()
	pasta.Network = Network{Mode: NetworkPasta, Interface: "eth0"}
	if err := Validate(pasta); err != nil {
		t.Fatalf("pasta interface eth0 should validate, got: %v", err)
	}

	caps := validApp()
	caps.Capabilities.Extra = []string{"NET_ADMIN", "CAP_NET_ADMIN"}
	if err := Validate(caps); err != nil {
		t.Fatalf("specific capabilities should validate, got: %v", err)
	}

	paths := validApp()
	paths.Mounts = []Mount{{Host: "/home/user/code", Container: "/work", Mode: MountRW}}
	paths.Keys = Keys{SSH: []string{"/home/user/.ssh/id_ed25519"}, GPG: []string{"/home/user/.gnupg/key.gpg"}}
	if err := Validate(paths); err != nil {
		t.Fatalf("metacharacter-free mount/key paths should validate, got: %v", err)
	}
}

func TestDefaultsFor(t *testing.T) {
	strict, _ := DefaultsFor(PresetStrict)
	if strict.Network.Mode != NetworkNone || strict.Display.Wayland != WaylandSecurityContext {
		t.Fatalf("strict defaults wrong: %+v", strict)
	}
	networked, _ := DefaultsFor(PresetNetworked)
	if networked.Network.Mode != NetworkPasta || networked.Display.Wayland != WaylandPassthrough {
		t.Fatalf("networked defaults wrong: %+v", networked)
	}
	if _, valid := DefaultsFor("nope"); valid {
		t.Fatal("unknown preset should return ok=false")
	}
	// Every preset's own defaults must themselves validate.
	for _, preset := range []string{PresetStrict, PresetStandard, PresetNetworked} {
		cfg, _ := DefaultsFor(preset)
		cfg.App.Name = "x"
		cfg.App.Image = "img" + digestPin
		if err := Validate(cfg); err != nil {
			t.Fatalf("preset %q defaults do not validate: %v", preset, err)
		}
	}
}

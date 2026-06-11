package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validApp() AppConfig {
	return AppConfig{
		SchemaVersion: SchemaVersion,
		App:           App{Name: "firefox", Image: "docker.io/library/firefox@sha256:abc", Preset: PresetStrict},
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
		cfg.App.Image = "img@sha256:abc"
		if err := Validate(cfg); err != nil {
			t.Fatalf("preset %q defaults do not validate: %v", preset, err)
		}
	}
}

func TestLoad_OK(t *testing.T) {
	cfg, err := Load(moduleFile(t, "examples/apps/firefox.toml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.App.Name != "firefox" || cfg.Network.Mode != NetworkNone {
		t.Fatalf("unexpected decode: %+v", cfg.App)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("example should validate: %v", err)
	}
}

// TestExampleApps is the "validate all shipped examples" check, by convention:
// examples/apps/*-broken.toml MUST be rejected, every other example MUST pass. It
// auto-discovers new examples and runs under `go test` (counts toward coverage, no
// subprocess) — the right home for this, vs a `go run`-per-file Make target.
func TestExampleApps(t *testing.T) {
	paths, err := filepath.Glob(moduleFile(t, "examples/apps/*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no example apps found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			cfg, loadErr := Load(path)
			if strings.HasSuffix(path, "-broken.toml") {
				// Rejection at parse or validation both count; never silent acceptance.
				if loadErr == nil && Validate(cfg) == nil {
					t.Fatal("expected rejection, but it validated clean")
				}
				return
			}
			if loadErr != nil {
				t.Fatalf("load: %v", loadErr)
			}
			if err := Validate(cfg); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}
}

// TestValidate_BrokenExample pins the joined-error behaviour (§3): one mis-edited
// file surfaces every problem in a single pass, not just the first.
func TestValidate_BrokenExample(t *testing.T) {
	cfg, err := Load(moduleFile(t, "examples/apps/firefox-broken.toml"))
	if err != nil {
		t.Fatalf("broken example uses only valid keys, should parse: %v", err)
	}
	err = Validate(cfg)
	if err == nil {
		t.Fatal("firefox-broken.toml must fail validation")
	}
	for _, want := range []string{
		`app.name "Firefox"`,
		`app.preset "paranoid"`,
		"app.autostart_workspace",
		`display.wayland "yolo"`,
		"network.ipv4_cidr",
		"network.ports",
		`mounts[0].mode "readonly"`,
		`theme.mode "dark"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q", want)
		}
	}
}

func TestLoad_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	const body = `schema_version = 1
[app]
name = "x"
image = "img@sha256:abc"
typpo = "drift"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("expected unknown-key error, got: %v", err)
	}
}

// moduleFile resolves a path relative to the module root (hzp/) from this test's
// location (hzp/internal/config → ../..).
func moduleFile(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join("..", "..", filepath.FromSlash(rel))
}

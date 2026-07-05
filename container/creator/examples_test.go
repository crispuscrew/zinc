package main

// These guard hzc's *shipped* example app definitions in examples/apps/ — the
// files the CLI, README, and Makefile reference. The pure schema/validation tests
// live in core/config; this validates the real files so a mis-edited example is
// caught in CI. Package main tests run with the working dir = hzc/, so the example
// paths are module-relative.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/hyprzinc/core/adapters/fs"
	"github.com/crispuscrew/hyprzinc/core/domain"
)

func TestLoad_OK(t *testing.T) {
	cfg, err := fs.Load("examples/apps/firefox.toml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.App.Name != "firefox" || cfg.Network.Mode != domain.NetworkNone {
		t.Fatalf("unexpected decode: %+v", cfg.App)
	}
	if err := domain.Validate(cfg); err != nil {
		t.Fatalf("example should validate: %v", err)
	}
}

// TestExampleApps is the "validate all shipped examples" check, by convention:
// examples/apps/*-broken.toml MUST be rejected, every other example MUST pass. It
// auto-discovers new examples and runs under `go test` (counts toward coverage).
func TestExampleApps(t *testing.T) {
	paths, err := filepath.Glob("examples/apps/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no example apps found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			cfg, loadErr := fs.Load(path)
			if strings.HasSuffix(path, "-broken.toml") {
				// Rejection at parse or validation both count; never silent acceptance.
				if loadErr == nil && domain.Validate(cfg) == nil {
					t.Fatal("expected rejection, but it validated clean")
				}
				return
			}
			if loadErr != nil {
				t.Fatalf("load: %v", loadErr)
			}
			if err := domain.Validate(cfg); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}
}

// TestValidate_BrokenExample pins the joined-error behaviour (§3): one mis-edited
// file surfaces every problem in a single pass, not just the first.
func TestValidate_BrokenExample(t *testing.T) {
	cfg, err := fs.Load("examples/apps/firefox-broken.toml")
	if err != nil {
		t.Fatalf("broken example uses only valid keys, should parse: %v", err)
	}
	err = domain.Validate(cfg)
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

package derived

import (
	"strings"
	"testing"
)

func installCfg(install string) AppConfig {
	return AppConfig{
		SchemaVersion: SchemaVersion,
		App: App{
			Name:    "hollywood",
			Image:   "docker.io/library/debian@sha256:abc",
			Install: install,
		},
		Display: Display{Wayland: WaylandSecurityContext},
		Network: Network{Mode: NetworkNone},
		Theme:   Theme{Mode: ThemeNone},
	}
}

func TestRunImageSelectsDerived(t *testing.T) {
	plain := installCfg("")
	if got := RunImage(plain); got != plain.App.Image {
		t.Fatalf("no install: run image should be the base, got %q", got)
	}
	withInstall := installCfg("apt-get install -y hollywood")
	if got := RunImage(withInstall); got != "hyprzinc/app-hollywood:local" {
		t.Fatalf("install set: run image should be the derived tag, got %q", got)
	}
	if HasInstall(plain) || !HasInstall(withInstall) {
		t.Fatal("HasInstall disagrees with the install line")
	}
}

func TestDerivedContainerfile(t *testing.T) {
	got := DerivedContainerfile(installCfg("  apt-get install -y hollywood  "))
	want := "FROM docker.io/library/debian@sha256:abc\nRUN apt-get install -y hollywood\n"
	if got != want {
		t.Fatalf("containerfile mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestDerivedContainerfileCollapsesMultilineInstall(t *testing.T) {
	got := DerivedContainerfile(installCfg("apt-get update\n\n  apt-get install -y foo  \n"))
	want := "FROM docker.io/library/debian@sha256:abc\nRUN apt-get update && apt-get install -y foo\n"
	if got != want {
		t.Fatalf("multi-line install should collapse to one && RUN:\n got %q\nwant %q", got, want)
	}
}

func TestBuildFingerprintChangesWithInputs(t *testing.T) {
	base := BuildFingerprint(installCfg("apt-get install -y hollywood"))
	if base == "" {
		t.Fatal("fingerprint must not be empty")
	}
	if base != BuildFingerprint(installCfg("apt-get install -y hollywood")) {
		t.Fatal("fingerprint must be deterministic for identical inputs")
	}
	if base == BuildFingerprint(installCfg("apt-get install -y sl")) {
		t.Fatal("a changed install line must change the fingerprint")
	}
	other := installCfg("apt-get install -y hollywood")
	other.App.Image = "docker.io/library/debian@sha256:def" // re-pinned base
	if base == BuildFingerprint(other) {
		t.Fatal("a changed base image must change the fingerprint")
	}
}

func TestInstallHint(t *testing.T) {
	cases := []struct{ image, want string }{
		{"docker.io/library/debian@sha256:abc", "apt-get"},
		{"docker.io/library/ubuntu:24.04", "apt-get"},
		{"docker.io/library/alpine@sha256:abc", "apk add"},
		{"registry.fedoraproject.org/fedora:40", "dnf install"},
		{"docker.io/library/archlinux:latest", "pacman"},
		{"docker.io/opensuse/leap:15", "zypper"},
		{"ghcr.io/some/mystery-image@sha256:abc", "package manager"},
	}
	for _, tcase := range cases {
		if got := InstallHint(tcase.image); !strings.Contains(got, tcase.want) {
			t.Errorf("InstallHint(%q) = %q; want it to contain %q", tcase.image, got, tcase.want)
		}
	}
}

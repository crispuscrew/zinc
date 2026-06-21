package domain

// Derived images (docs §5.5, §9.1). When an app sets app.install, HyprZinc builds a
// small derived image — `FROM <app.image>` plus one `RUN <install>` layer — and
// runs the app from that instead of the bare base. This is the "quick setup" path:
// take a stock distro image and `apt`/`apk`/`dnf` the program you want, without
// authoring a Containerfile by hand.
//
// The base FROM inherits app.image, which §5.5 forces to be digest-pinned (or a
// trusted-* local tag), so the derived image is built from a known base. These are
// pure policy functions: which image runs, what its Containerfile says, and how to
// tell a fresh build from a stale one. The actual `podman build` is the podman
// adapter (core/adapters/podman); the build trigger is the app layer (core/app).

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// BuildLabel is the OCI label that stamps a derived image with the fingerprint of
// the inputs it was built from, so a launch can tell a fresh image from a stale
// one without any extra on-disk state.
const BuildLabel = "hyprzinc.build"

// HasInstall reports whether cfg builds a derived image (app.install is set). When
// false the app runs straight from app.image and no build ever happens.
func HasInstall(cfg AppConfig) bool {
	return strings.TrimSpace(cfg.App.Install) != ""
}

// RunImage is the image a container actually runs from: the locally built derived
// image when app.install is set, otherwise app.image itself. The podman adapter
// uses this for the trailing image argument, so an install app transparently runs
// the derived image once it has been built (the app layer ensures that first).
func RunImage(cfg AppConfig) string {
	if HasInstall(cfg) {
		return DerivedImageRef(cfg.App.Name)
	}
	return cfg.App.Image
}

// DerivedImageRef is the local tag of an app's derived image. It is bare (no
// "localhost/" prefix) on purpose: a registry-qualified local tag has tripped
// image resolution before (the netfilter-image lesson, §5.5), and this image is
// only ever referenced locally, never pulled.
func DerivedImageRef(name string) string {
	return "hyprzinc/app-" + name + ":local"
}

// DerivedContainerfile renders the Containerfile for an app's derived image: the
// pinned base plus a single RUN layer carrying the install line. The install line
// runs through the image's own /bin/sh (Containerfile shell form), so a distro
// package-manager invocation works exactly as the user would type it at a shell
// (e.g. "apt-get update && apt-get install -y hollywood"). It is fed to `podman
// build` on stdin, so no temp file and no host build context are needed.
func DerivedContainerfile(cfg AppConfig) string {
	return "FROM " + cfg.App.Image + "\nRUN " + strings.TrimSpace(cfg.App.Install) + "\n"
}

// BuildFingerprint identifies a derived image's inputs — the base image and the
// install line. It is written as the BuildLabel value at build time; a launch
// rebuilds only when the live image's label differs (or the image is missing), so
// an unchanged app reuses its image and a re-pinned base or edited install line
// takes effect on the next run automatically.
func BuildFingerprint(cfg AppConfig) string {
	sum := sha256.Sum256([]byte(cfg.App.Image + "\n" + strings.TrimSpace(cfg.App.Install)))
	return hex.EncodeToString(sum[:])
}

// InstallHint suggests the package-manager invocation for an app's base image,
// keyed off well-known image-name families, so the form can show the right syntax
// (apt for debian/ubuntu, apk for alpine, dnf for fedora/rhel, …). It is best-
// effort UI sugar over the image name only — it never constrains what may be typed
// into app.install, and falls back to a generic note for unrecognised images.
func InstallHint(image string) string {
	img := strings.ToLower(image)
	switch {
	case containsAny(img, "ubuntu", "debian", "/buildpack-deps", "mint"):
		return "apt-get update && apt-get install -y <pkg>"
	case containsAny(img, "alpine"):
		return "apk add --no-cache <pkg>"
	case containsAny(img, "fedora", "rockylinux", "almalinux", "/rhel", "centos", "oraclelinux"):
		return "dnf install -y <pkg>"
	case containsAny(img, "archlinux", "/arch:", "manjaro"):
		return "pacman -Sy --noconfirm <pkg>"
	case containsAny(img, "opensuse", "/suse", "sles"):
		return "zypper install -y <pkg>"
	default:
		return "<your image's package manager> install <pkg>"
	}
}

// containsAny reports whether str contains any of subs.
func containsAny(str string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(str, sub) {
			return true
		}
	}
	return false
}

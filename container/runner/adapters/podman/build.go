package podman

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
)

// Builder implements ports.ImageBuilder against podman: it builds an app's derived
// image (FROM ImageMeta.Image + the install layer) and reads back the fingerprint
// label.
type Builder struct{}

// ImageBuildArgs is the `podman build` argv for an app's derived image: tag it with
// the local ref, stamp the fingerprint label, and read the Containerfile from stdin
// with an empty context (the trailing "-"). The install layer only needs the base
// image and network, never host files, so the context is deliberately empty. Pure.
func ImageBuildArgs(cfg schema.AppConfig) []string {
	return []string{
		"build",
		"-t", derived.DerivedImageRef(cfg.AppNameID),
		"--label", derived.BuildLabel + "=" + derived.BuildFingerprint(cfg),
		"-", // Containerfile on stdin, no build context
	}
}

// Build builds the derived image unconditionally (the explicit-rebuild path). The
// Containerfile is fed on stdin; output is surfaced on failure so a broken install
// line is debuggable.
func (Builder) Build(cfg schema.AppConfig) error {
	proc := exec.Command("podman", ImageBuildArgs(cfg)...)
	proc.Stdin = strings.NewReader(derived.DerivedContainerfile(cfg))
	if out, err := proc.CombinedOutput(); err != nil {
		return fmt.Errorf("build image for %s: %s", cfg.AppNameID, strings.TrimSpace(string(out)))
	}
	return nil
}

// Fingerprint reads the build label off a local image, or returns an error when the
// image does not exist - either case means the app layer should (re)build. The label
// is empty for an image built outside Zinc, which also triggers a rebuild.
func (Builder) Fingerprint(ref string) (string, error) {
	out, err := exec.Command("podman", "image", "inspect", ref,
		"--format", fmt.Sprintf(`{{index .Config.Labels %q}}`, derived.BuildLabel)).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

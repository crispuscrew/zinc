package podman

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/crispuscrew/hyprzinc/core/ports"
)

// Resolver implements ports.ImageResolver: it finds images and pins tags to digests
// using podman only (no skopeo), so it works on a minimal host and at first setup.
type Resolver struct{}

// run is the single podman exec point, swappable in tests.
var run = func(args ...string) ([]byte, error) {
	return exec.Command("podman", args...).Output()
}

// Search queries configured registries for term — a thin wrapper over `podman
// search` whose output is parsed into Results.
func (Resolver) Search(term string) ([]ports.Result, error) {
	if strings.TrimSpace(term) == "" {
		return nil, fmt.Errorf("image: empty search term")
	}
	out, err := run("search", "--format", "{{.Name}}\t{{.Description}}", term)
	if err != nil {
		return nil, fmt.Errorf("image: podman search %q: %w", term, err)
	}
	return parseSearch(string(out)), nil
}

func parseSearch(out string) []ports.Result {
	var res []ports.Result
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, desc, _ := strings.Cut(line, "\t")
		res = append(res, ports.Result{Name: strings.TrimSpace(name), Description: strings.TrimSpace(desc)})
	}
	return res
}

// Resolve turns a tag/name (e.g. "alpine:3.20") into a digest-pinned reference
// ("docker.io/library/alpine@sha256:..."), satisfying §5.5. An already-pinned ref
// is returned unchanged. Otherwise it pulls the image — podman's canonical way to
// learn a manifest digest without skopeo — then reads RepoDigests.
func (Resolver) Resolve(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("image: empty reference")
	}
	if strings.Contains(ref, "@sha256:") {
		return ref, nil // already pinned
	}
	if _, err := run("pull", ref); err != nil {
		return "", fmt.Errorf("image: podman pull %q: %w", ref, err)
	}
	out, err := run("image", "inspect", ref, "--format", "{{range .RepoDigests}}{{println .}}{{end}}")
	if err != nil {
		return "", fmt.Errorf("image: inspect %q: %w", ref, err)
	}
	return pickRepoDigest(ref, string(out))
}

// pickRepoDigest chooses the digest reference whose repository matches ref from
// podman's RepoDigests, falling back to the first digest entry. Pure (unit-tested).
func pickRepoDigest(ref, repoDigestsOut string) (string, error) {
	repo := repoOf(ref)
	var first string
	for _, line := range strings.Split(strings.TrimSpace(repoDigestsOut), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "@sha256:") {
			continue
		}
		if first == "" {
			first = line
		}
		if repoOf(line) == repo {
			return line, nil
		}
	}
	if first != "" {
		return first, nil
	}
	return "", fmt.Errorf("image: no digest found for %q (RepoDigests empty — registry may not expose one)", ref)
}

// repoOf strips a :tag and @digest from a reference, leaving the repository path for
// comparison. A registry :port (before the last '/') is preserved.
func repoOf(ref string) string {
	if atIdx := strings.IndexByte(ref, '@'); atIdx >= 0 {
		ref = ref[:atIdx]
	}
	if slashIdx := strings.LastIndexByte(ref, '/'); slashIdx >= 0 {
		if colonIdx := strings.IndexByte(ref[slashIdx:], ':'); colonIdx >= 0 {
			ref = ref[:slashIdx+colonIdx]
		}
	} else if colonIdx := strings.IndexByte(ref, ':'); colonIdx >= 0 {
		ref = ref[:colonIdx]
	}
	return ref
}

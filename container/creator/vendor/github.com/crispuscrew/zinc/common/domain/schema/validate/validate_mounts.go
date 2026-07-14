package validate

import (
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Validates the ':'-delimited podman mount specs — Volumes, Configs, Keys — plus
// Capabilities. host:container:opts is ':'-split, so ':'/','/whitespace in a path
// shifts podman's fields (e.g. claim "ro" but mount "rw"); every path is screened.

// checkVolume: container path, host source (when HostMounted), size-limit sanity.
func checkVolume(index int, volume schema.Volume, add addFunc) {
	checkInner("Volumes", index, volume.InnerMount, add)
	if volume.HostMounted {
		switch {
		case strings.TrimSpace(volume.HostMount) == "":
			add("Volumes[%d].HostMount: required when HostMounted=true", index)
		case hasUnsafe(volume.HostMount) || strings.ContainsAny(volume.HostMount, ":,"):
			add("Volumes[%d].HostMount %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", index, volume.HostMount)
		}
	}
	checkSizeLimit("Volumes", index, volume, add)
}

// checkConfig: like a Volume but bundle-relative (apps/<name>/configs/) — no absolute
// path, leading '/', or '..'; HostMounted is ignored (a Config is always bundle-sourced).
func checkConfig(index int, configMount schema.Volume, add addFunc) {
	checkInner("Configs", index, configMount.InnerMount, add)
	source := configMount.HostMount
	switch {
	case strings.TrimSpace(source) == "":
		add("Configs[%d].HostMount: must name a path under the app bundle (apps/<name>/configs/)", index)
	case hasUnsafe(source) || strings.ContainsAny(source, ":,"):
		add("Configs[%d].HostMount %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", index, source)
	case strings.HasPrefix(source, "/"):
		add("Configs[%d].HostMount %q: must be relative to the app bundle, not an absolute path", index, source)
	case hasDotDot(source):
		add("Configs[%d].HostMount %q: must not escape the app bundle (no '..' segments)", index, source)
	}
	checkSizeLimit("Configs", index, configMount, add)
}

// checkKeys: known Type + field-shift-safe Path (mounted path:dest:ro).
func checkKeys(keys []schema.Key, add addFunc) {
	for index, keyEntry := range keys {
		switch keyEntry.Type {
		case schema.SSH, schema.GPG:
		case "":
			add("Keys[%d].Type: must be set (SSH|GPG)", index)
		default:
			add("Keys[%d].Type %q: must be SSH|GPG", index, keyEntry.Type)
		}
		switch {
		case strings.TrimSpace(keyEntry.Path) == "":
			add("Keys[%d].Path: must not be empty", index)
		case hasUnsafe(keyEntry.Path) || strings.ContainsAny(keyEntry.Path, ":,"):
			add("Keys[%d].Path %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", index, keyEntry.Path)
		}
	}
}

// checkCapabilities: ALL is forbidden; each entry must match the capability charset.
func checkCapabilities(capabilities []string, add addFunc) {
	for index, capability := range capabilities {
		bare := strings.TrimPrefix(strings.ToUpper(capability), "CAP_")
		switch {
		case bare == "ALL":
			add("Capabilities[%d] %q: granting ALL capabilities is forbidden (add only the specific caps an app needs)", index, capability)
		case !capRE.MatchString(capability):
			add("Capabilities[%d] %q: only an (optional) CAP_ prefix then [A-Z_] allowed (e.g. NET_ADMIN or CAP_NET_ADMIN)", index, capability)
		}
	}
}

// checkInner: container-side mount path — non-empty, no ':'/','/whitespace.
func checkInner(list string, index int, inner string, add addFunc) {
	switch {
	case strings.TrimSpace(inner) == "":
		add("%s[%d].InnerMount: must not be empty", list, index)
	case hasUnsafe(inner) || strings.ContainsAny(inner, ":,"):
		add("%s[%d].InnerMount %q: must not contain ':', ',', or whitespace (it shifts podman's -v fields)", list, index, inner)
	}
}

// checkSizeLimit: reject SizeLimited with non-positive MiB (a no-op that reads as applied).
func checkSizeLimit(list string, index int, volume schema.Volume, add addFunc) {
	if volume.SizeLimited && volume.SizeLimitMiB <= 0 {
		add("%s[%d]: SizeLimited is set but SizeLimitMiB is %d (must be > 0, or clear SizeLimited)", list, index, volume.SizeLimitMiB)
	}
}

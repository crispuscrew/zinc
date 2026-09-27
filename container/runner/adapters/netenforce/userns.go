package netenforce

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
)

// Freeze the engine identity (including failures) for every plan in this process.
var podmanUserNamespace = sync.OnceValues(probeUserNamespace)

func probeUserNamespace() (uint64, error) {
	return engineUserNamespace(podmanOutput, func() (string, error) { return os.Readlink("/proc/self/ns/user") })
}

func engineUserNamespace(command func(...string) (string, error), current func() (string, error)) (uint64, error) {
	mode, err := command("info", "--format", "{{.Host.ServiceIsRemote}} {{.Host.Security.Rootless}}")
	if err != nil {
		return 0, fmt.Errorf("probe Podman mode: %w", err)
	}
	var output string
	switch strings.TrimSpace(mode) {
	case "false true":
		output, err = command("unshare", "readlink", "/proc/self/ns/user")
	case "false false":
		// Local rootful Podman inherits the caller's userns; it refuses unshare.
		output, err = current()
	default:
		return 0, fmt.Errorf("cannot bind local namespace paths to Podman mode %q", mode)
	}
	if err != nil {
		return 0, fmt.Errorf("probe Podman user namespace: %w", err)
	}
	return parseUserNamespace(output)
}

func parseUserNamespace(output string) (uint64, error) {
	value, prefix := strings.CutPrefix(strings.TrimSpace(output), "user:[")
	value, suffix := strings.CutSuffix(value, "]")
	inode, err := strconv.ParseUint(value, 10, 64)
	if !prefix || !suffix || err != nil || inode == 0 {
		return 0, fmt.Errorf("invalid Podman user namespace identity %q", output)
	}
	return inode, nil
}

func (enforcer Enforcer) userNamespaceMode(manifest provision.Manifest) (string, error) {
	resolve := enforcer.UserNamespace
	if resolve == nil {
		resolve = podmanUserNamespace
	}
	inode, err := resolve()
	if err != nil {
		return "", fmt.Errorf("resolve Podman user namespace: %w", err)
	}
	if inode == 0 || manifest.UserInode == 0 {
		return "", fmt.Errorf("Podman and provisioned user namespace identities must be nonzero")
	}
	if inode == manifest.UserInode {
		// Podman's current namespace cannot be re-entered with setns(CLONE_NEWUSER).
		// Helper preflight still verifies the actual user AND network inode.
		return "host", nil
	}
	return "ns:" + manifest.UserNamespace, nil
}

package network

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"
)

func namespace(path string, inode uint64, kind string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("provisioned %s namespace: %w", kind, err)
	}
	stat, valid := info.Sys().(*syscall.Stat_t)
	if !valid || inode == 0 || stat.Ino != inode {
		return fmt.Errorf("provisioned %s namespace identity changed", kind)
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(path, &filesystem); err != nil {
		return err
	}
	const namespaceFilesystem = 0x6e736673
	if filesystem.Type != namespaceFilesystem {
		return fmt.Errorf("%s is not an nsfs namespace", path)
	}
	current, err := os.Stat("/proc/self/ns/" + kind)
	if err != nil {
		return err
	}
	if os.SameFile(info, current) {
		return fmt.Errorf("refusing current host %s namespace", kind)
	}
	return nil
}

func Enter(manifest Manifest) []string {
	return []string{"nsenter", "--user=" + manifest.UserNamespace, "--net=" + manifest.NetworkNamespace, "--preserve-credentials"}
}

func Quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func ShellJoin(argv []string) string {
	var words []string
	for _, value := range argv {
		words = append(words, Quote(value))
	}
	return strings.Join(words, " ")
}

// Preflight checks the namespace actually entered, not merely a mutable pathname.
// ip/nft are the provisioned namespace's existing tools; no host topology changes.
func Preflight(manifest Manifest) string {
	return namespaceIdentity(manifest) + checkDevices(manifest)
}

func namespaceIdentity(manifest Manifest) string {
	var result strings.Builder
	fmt.Fprintf(&result, "test \"$(readlink /proc/self/ns/net)\" = %s\n", Quote(fmt.Sprintf("net:[%d]", manifest.NetworkInode)))
	fmt.Fprintf(&result, "test \"$(readlink /proc/self/ns/user)\" = %s\n", Quote(fmt.Sprintf("user:[%d]", manifest.UserInode)))
	return result.String()
}

func checkDevices(manifest Manifest) string {
	var result strings.Builder
	seen := map[string]bool{}
	for _, entry := range manifest.Topology.Interfaces {
		seen[entry.Device] = true
	}
	for _, peer := range manifest.Topology.Peers {
		for _, entry := range peer.Interfaces {
			seen[entry.Device] = true
		}
	}
	for _, host := range manifest.Topology.Hosts {
		seen[host.Device] = true
	}
	for _, device := range manifest.Topology.ExternalInterfaces {
		seen[device] = true
	}
	for _, device := range sortedDevices(seen) {
		fmt.Fprintf(&result, "ip link show dev %s >/dev/null\n", Quote(device))
	}
	return result.String()
}

func sortedDevices(seen map[string]bool) []string {
	var result []string
	for device := range seen {
		result = append(result, device)
	}
	sort.Strings(result)
	return result
}

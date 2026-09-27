package network

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/nftrules"
)

func closeFirewall() string {
	return "printf '%s' " + Quote(nftrules.DenyAll()) + " | nft -f -"
}

// ApplyScript closes first, then atomically loads stdin. A failed load or probe
// leaves default deny. The successful initializer leaves the installed policy.
func ApplyScript(manifest Manifest) string {
	return "set -eu\n" + namespaceIdentity(manifest) + closeFirewall() + "\n" +
		"trap " + Quote(closeFirewall()) + " EXIT\n" +
		checkDevices(manifest) + "nft -f -\ntrap - EXIT\n"
}

// RunScript keeps a supervisor alive to close the guest packet path on failure,
// normal exit, or termination. The actual guest is never started before nft.
func RunScript(manifest Manifest, argv []string) string {
	cleanup := "status=$?; trap - EXIT; closed=0; " + closeFirewall() + " || closed=$?; " +
		"if [ -n \"${child:-}\" ]; then if [ \"$closed\" -ne 0 ]; then kill -KILL \"$child\" 2>/dev/null || true; " +
		"else kill \"$child\" 2>/dev/null || true; fi; wait \"$child\" 2>/dev/null || true; fi; " +
		"if [ \"$closed\" -ne 0 ]; then exit \"$closed\"; fi; exit \"$status\""
	return "set -eu\n" + namespaceIdentity(manifest) + closeFirewall() + "\n" +
		"child=''\ntrap " + Quote(cleanup) + " EXIT\ntrap 'exit 143' TERM\ntrap 'exit 130' INT\ntrap 'exit 129' HUP\n" +
		checkDevices(manifest) + "nft -f -\nsetpriv --no-new-privs --bounding-set=-all --inh-caps=-all --ambient-caps=-all -- " + ShellJoin(argv) +
		" &\nchild=$!\nstatus=0\nwait \"$child\" || status=$?\nchild=''\nexit \"$status\"\n"
}

func CloseScript(manifest Manifest) string {
	return "set -eu\n" + namespaceIdentity(manifest) + closeFirewall() + "\n"
}

func ResolvConf(manifest Manifest) string {
	var result strings.Builder
	for _, address := range manifest.DNSProxyAddresses {
		fmt.Fprintf(&result, "nameserver %s\n", address)
	}
	return result.String()
}

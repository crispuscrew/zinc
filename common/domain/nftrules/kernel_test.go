package nftrules

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/network"
)

// Run only in a disposable, disconnected user-owned namespace. --check asks
// the kernel to validate the batch without installing tables or changing links.
func TestNftKernelCheck(t *testing.T) {
	if os.Getenv("ZINC_NFT_CHECK") != "1" {
		t.Skip("opt-in nft --check in an isolated test namespace")
	}
	resolved := fixture()
	addPeer(&resolved)
	resolved.Config.NetworkMeta.RulesByPriority[0].Protocols = nil
	for _, mode := range []string{network.Container, network.VirtualMachine} {
		resolved.Topology.Mode = mode
		body := render(t, resolved)
		// A no-network test container has only lo. Replace only the netdev hook
		// device for the syntax/kernel check; no TAP is created or exercised.
		body = strings.ReplaceAll(body, `hook ingress device "eth0"`, `hook ingress device "lo"`)
		command := exec.Command("nft", "--check", "--file", "-")
		command.Stdin = strings.NewReader(body)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s nft check: %v\n%s\n%s", mode, err, output, body)
		}
	}
}

package network_test

import (
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/nftrules"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

// A correctly identified test namespace with stale grants must be quarantined
// even when a provisioned device disappeared. Never suppress this regression.
func TestLivePreflightFailureClosesPolicy(test *testing.T) {
	lab := newLab(test, false)
	startServer(test, lab.server, "tcp", lab.server4, 8080)
	allow := schema.NetworkRule{From: schema.NetworkPeer{Type: schema.NetworkPeerAny},
		To: schema.NetworkPeer{Type: schema.NetworkPeerAny}, Protocols: []schema.NetworkProtocol{schema.NetworkTCP}}
	lab.apply(test, []schema.NetworkRule{allow}, []schema.NetworkRule{allow})
	verifyProbe(test, runProbe(test, lab.client, "tcp", lab.client4, lab.server4, nextPort(), 8080), true)
	before := packets(counters(test, lab), "both endpoints authorized")
	_, manifest := lab.fixture(test, nil, nil)
	manifest.Topology.Interfaces[0].Device = "missing0"
	output, err := command(lab.policyNS(), nftrules.DenyAll(), "sh", "-c", provision.ApplyScript(manifest))
	if err == nil {
		test.Fatal("missing interface did not stop preflight")
	}
	test.Logf("preflight failed in correctly pinned namespace: %s", output)
	result := runProbe(test, lab.client, "tcp", lab.client4, lab.server4, nextPort(), 8080)
	if result.Success {
		after := packets(counters(test, lab), "both endpoints authorized")
		test.Fatalf("FAIL-CLOSED BUG: fresh TCP echo succeeds after missing-device preflight failure; prior permissive policy survived; admissions before=%d after=%d", before, after)
	}
	verifyProbe(test, result, false)
	if packets(counters(test, lab), "default") == 0 {
		test.Fatal("preflight failure did not install deny-all")
	}
}

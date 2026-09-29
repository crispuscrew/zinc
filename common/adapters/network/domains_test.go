package network

import (
	"net/netip"
	"strings"
	"testing"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestPeerDomainPolicyRequiresItsOwnApprovedResolver(t *testing.T) {
	cfg, manifest := manifestFixture()
	manifest.Topology.Peers = []domain.Peer{{AppNameID: "server", Interfaces: []domain.Attachment{{InterfaceID: "main", Device: "eth0", MAC: "02:00:00:00:00:02", Addresses: []string{"10.0.0.3"}}},
		Policy: schema.NetworkMeta{Interfaces: []schema.NetworkInterface{{ID: "main"}}, DNS: schema.DNSMeta{ResolversByPriority: []schema.DNSResolver{{Protocol: schema.DNSHTTPS, Endpoint: "9.9.9.9"}}},
			RulesByPriority: []schema.NetworkRule{{From: schema.NetworkPeer{Type: schema.NetworkPeerAny}, To: schema.NetworkPeer{Type: schema.NetworkPeerSelf}, Domains: []string{"example.org"}, AllowAllExcept: true}}},
	}}
	if _, err := Resolve(cfg, manifest, nil); err == nil || !strings.Contains(err.Error(), "server rule[0]") {
		t.Fatalf("peer deny silently omitted: %v", err)
	}
	lookup := func(meta schema.DNSMeta, name string) ([]netip.Addr, error) {
		if name != "example.org" || meta.ResolversByPriority[0].Endpoint != "9.9.9.9" {
			t.Fatal("owner DNS was substituted for peer DNS")
		}
		return []netip.Addr{netip.MustParseAddr("10.0.0.3")}, nil
	}
	plan, err := Resolve(cfg, manifest, lookup)
	if err != nil || len(plan.Resolved.Domains["server"][0]) != 1 {
		t.Fatalf("peer domain result lost: %v", err)
	}
}

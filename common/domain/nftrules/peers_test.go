package nftrules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func protocolNumber(number int) string { return fmt.Sprintf("meta l4proto %d", number) }

func addPeer(resolved *network.Resolved) {
	resolved.Topology.Peers = []network.Peer{{AppNameID: "server",
		Interfaces: []network.Attachment{{InterfaceID: "service", Device: "eth0", MAC: "02:00:00:00:00:02", Addresses: []string{"10.10.0.3", "fd00::3"}}},
		Policy: schema.NetworkMeta{Interfaces: []schema.NetworkInterface{{ID: "service"}}, RulesByPriority: []schema.NetworkRule{{
			From: schema.NetworkPeer{Type: schema.NetworkPeerApp, AppNameID: "client"}, To: schema.NetworkPeer{Type: schema.NetworkPeerSelf},
		}}},
	}}
}

func TestBothEndpointPoliciesBeforeAcceptance(t *testing.T) {
	resolved := fixture()
	addPeer(&resolved)
	resolved.Config.NetworkMeta.RulesByPriority[0].To = schema.NetworkPeer{Type: schema.NetworkPeerApp, AppNameID: "server", Interface: "service"}
	result := render(t, resolved)
	start := strings.Index(result, " chain output {")
	chain := result[start:]
	owner, peer, accept := strings.Index(chain, "jump owner_output"), strings.Index(chain, "jump peer_0_output"), strings.Index(chain, "both endpoints authorized")
	if owner < 0 || peer < owner || accept < peer {
		t.Fatalf("endpoint conjunction missing: %s", chain)
	}
	if !strings.Contains(result, `counter return comment "client rule[0]`) || !strings.Contains(result, `counter return comment "server rule[0]`) {
		t.Fatal("policy accept must return to other endpoint check")
	}
	resolved.Topology.Peers[0].Policy.RulesByPriority = nil
	if !strings.Contains(render(t, resolved), `counter drop comment "server default"`) {
		t.Fatal("closed peer gained an implicit allowance")
	}
}

func TestInternetExcludesHostAppsAndSpecialAddresses(t *testing.T) {
	resolved := fixture()
	addPeer(&resolved)
	result := render(t, resolved)
	for _, value := range []string{"10.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.168.0.0/16", "8.8.4.4/32", "10.10.0.3/32", "fd00::3/128", "ip daddr != @nonpublic_ip", "ip6 daddr != @nonpublic_ip6"} {
		if !strings.Contains(result, value) {
			t.Errorf("Internet boundary missing %s", value)
		}
	}
}

func TestPeerScopesAndFamilyIntersection(t *testing.T) {
	for _, kind := range []schema.NetworkPeerType{schema.NetworkPeerSelf, schema.NetworkPeerAnyApp, schema.NetworkPeerHost, schema.NetworkPeerAny} {
		resolved := fixture()
		resolved.Config.NetworkMeta.RulesByPriority[0].To.Type = kind
		render(t, resolved)
	}
	resolved := fixture()
	rule := &resolved.Config.NetworkMeta.RulesByPriority[0]
	rule.From.Filter.IPv4CIDR = []string{"10.10.0.0/24"}
	rule.To.Filter.IPv4CIDR = []string{"1.1.1.0/24"}
	result := render(t, resolved)
	if !strings.Contains(result, "ip saddr { 10.10.0.0/24 }") || !strings.Contains(result, "ip daddr { 1.1.1.0/24 }") {
		t.Fatal("directional address filters lost")
	}
	if strings.Contains(result, `client rule[0] ip6`) {
		t.Fatal("v4-only filters leaked into v6")
	}
}

func TestTAPFiltersGuestPacketsAndSpoofing(t *testing.T) {
	resolved := fixture()
	resolved.Topology.Mode = network.VirtualMachine
	resolved.Config.Type = schema.ZincVirtualization
	resolved.Topology.Interfaces[0].Device = "ztap0"
	result := render(t, resolved)
	for _, value := range []string{"jump owner_forward", `iifname "ztap0"`, `oifname "ztap0"`, "table netdev zinc_link", "ether saddr != 02:00:00:00:00:01", "arp saddr ether", "endpoint spoof", "ct mark"} {
		if !strings.Contains(result, value) {
			t.Errorf("missing guest packet enforcement: %s", value)
		}
	}
	if strings.Contains(result, "jump owner_output") || strings.Contains(result, "jump owner_input") {
		t.Fatal("QEMU sockets must not get guest allowances")
	}
}

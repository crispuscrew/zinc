package nftrules

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func fixture() network.Resolved {
	return network.Resolved{
		Config: schema.AppConfig{AppNameID: "client", Type: schema.ZincContainer, NetworkMeta: schema.NetworkMeta{
			Interfaces:      []schema.NetworkInterface{{ID: "main"}},
			RulesByPriority: []schema.NetworkRule{{From: schema.NetworkPeer{Type: schema.NetworkPeerSelf}, To: schema.NetworkPeer{Type: schema.NetworkPeerInternet}, Protocols: []schema.NetworkProtocol{schema.NetworkTCP}}},
		}},
		Topology: network.Topology{Mode: network.Container,
			Interfaces:         []network.Attachment{{InterfaceID: "main", Device: "eth0", MAC: "02:00:00:00:00:01", Addresses: []string{"10.10.0.2", "fd00::2"}}},
			Hosts:              []network.Host{{Interface: "host0", Device: "eth0", Addresses: []string{"10.10.0.1", "8.8.4.4", "fd00::1"}}},
			ExternalInterfaces: []string{"eth0"},
		},
	}
}

func render(t *testing.T, resolved network.Resolved) string {
	t.Helper()
	result, err := RenderResolved(resolved)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRenderCompatibilityFailsClosed(t *testing.T) {
	if Render(schema.AppConfig{}) != "" {
		t.Fatal("no NIC should need no rules")
	}
	resolved := fixture()
	if got := Render(resolved.Config); got != DenyAll() {
		t.Fatal("unresolved config granted connectivity")
	}
	for _, deny := range []bool{false, true} {
		resolved.Config.NetworkMeta.RulesByPriority[0].AllowAllExcept = deny
		if !DefaultDrop(resolved.Config) {
			t.Fatal("default changed with rule polarity")
		}
		result := render(t, resolved)
		if strings.Contains(result, "policy accept") || strings.Count(result, "policy drop;") != 3 {
			t.Fatalf("hooks not closed: %s", result)
		}
	}
}

func TestRenderOrderedRulesAndBothHooks(t *testing.T) {
	resolved := fixture()
	deny := resolved.Config.NetworkMeta.RulesByPriority[0]
	deny.AllowAllExcept = true
	deny.To.Filter = schema.NetworkPeerFilter{IPv4CIDR: []string{"1.1.1.0/24"}, Ports: []int{443}}
	allow := resolved.Config.NetworkMeta.RulesByPriority[0]
	for _, rules := range [][]schema.NetworkRule{{deny, allow}, {allow, deny}, {deny}} {
		resolved.Config.NetworkMeta.RulesByPriority = rules
		result := render(t, resolved)
		for _, hook := range []string{"input", "output"} {
			start := strings.Index(result, "chain owner_"+hook)
			body := result[start:]
			first := strings.Index(body, "client rule[0]")
			last := strings.Index(body, "client default")
			if first < 0 || last < first {
				t.Fatal("rule priority/default lost")
			}
			if len(rules) > 1 && strings.Index(body, "client rule[1]") < first {
				t.Fatal("priority reordered")
			}
		}
	}
}

func TestRenderSourceDestinationAndProtocols(t *testing.T) {
	for protocol, number := range network.ProtocolNumbers {
		resolved := fixture()
		rule := &resolved.Config.NetworkMeta.RulesByPriority[0]
		rule.Protocols = []schema.NetworkProtocol{protocol}
		if protocol == schema.NetworkTCP || protocol == schema.NetworkUDP || protocol == schema.NetworkSCTP {
			rule.From.Filter.Ports, rule.To.Filter.Ports = []int{12345}, []int{443}
		}
		result := render(t, resolved)
		if !strings.Contains(result, protocolNumber(number)) {
			t.Errorf("missing protocol %s", protocol)
		}
		if len(rule.From.Filter.Ports) > 0 && (!strings.Contains(result, "sport { 12345 }") || !strings.Contains(result, "dport { 443 }")) {
			t.Fatal("source/destination port roles lost")
		}
	}
}

func TestRenderReplacesOnlyOwnedTables(t *testing.T) {
	result := render(t, fixture())
	if !strings.HasPrefix(result, "table inet zinc\ndelete table inet zinc\n") || strings.Contains(result, "flush ruleset") {
		t.Fatal("non-atomic or overbroad replacement")
	}
	if strings.Contains(result, `oif "lo" accept`) || strings.Contains(result, `iif "lo" accept`) {
		t.Fatal("unconditional loopback exposure")
	}
	if render(t, fixture()) != result {
		t.Fatal("nondeterministic render")
	}
}

package nftrules

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestDNSMetadataNeverBecomesBroadFirewallAllowance(t *testing.T) {
	resolved := fixture()
	resolved.Config.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSHTTPS, Endpoint: "9.9.9.9", Path: "/dns-query"}}
	result := render(t, resolved)
	for _, forbidden := range []string{"9.9.9.9", "53, 853", "dnat", "declared dns"} {
		if strings.Contains(result, forbidden) {
			t.Errorf("implicit DNS allowance: %s", forbidden)
		}
	}
}

func TestDomainsRequireApprovedAnswersForBothVerdicts(t *testing.T) {
	for _, deny := range []bool{true, false} {
		resolved := fixture()
		resolved.Config.NetworkMeta.RulesByPriority[0].Domains = []string{"example.org"}
		resolved.Config.NetworkMeta.RulesByPriority[0].AllowAllExcept = deny
		resolved.Config.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSTLS, Endpoint: "1.1.1.1"}}
		if _, err := RenderResolved(resolved); err == nil {
			t.Fatal("unresolved domain became wildcard/empty denial")
		}
		resolved.Domains = network.DomainSets{"client": {0: {"1.1.1.2", "2606:4700::1112"}}}
		result := render(t, resolved)
		if !strings.Contains(result, "ip daddr { 1.1.1.2/32 }") || !strings.Contains(result, "ip6 daddr { 2606:4700::1112/128 }") {
			t.Fatal("domain answers not destination-scoped")
		}
	}
}

func TestMalformedInputsFailBeforeRendering(t *testing.T) {
	mutations := []func(*network.Resolved){
		func(value *network.Resolved) {
			value.Config.NetworkMeta.RulesByPriority[0].Protocols = []schema.NetworkProtocol{"BOGUS"}
		},
		func(value *network.Resolved) { value.Config.NetworkMeta.RulesByPriority[0].To.Interface = "missing" },
		func(value *network.Resolved) {
			value.Config.NetworkMeta.RulesByPriority[0].To.Type = schema.NetworkPeerApp
			value.Config.NetworkMeta.RulesByPriority[0].To.AppNameID = "missing"
		},
		func(value *network.Resolved) {
			value.Config.NetworkMeta.RulesByPriority[0].From.Filter.Ports = []int{65536}
		},
		func(value *network.Resolved) {
			value.Config.NetworkMeta.RulesByPriority[0].From.Filter.IPv4CIDR = []string{"::/0"}
		},
		func(value *network.Resolved) { value.Topology.Interfaces[0].Device = "eth0; accept" },
		func(value *network.Resolved) { value.Topology.Interfaces[0].Addresses = []string{"0.0.0.0/0"} },
	}
	for index, mutate := range mutations {
		resolved := fixture()
		mutate(&resolved)
		if result, err := RenderResolved(resolved); err == nil || result != "" {
			t.Errorf("case %d rendered invalid policy", index)
		}
	}
}

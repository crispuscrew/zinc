package validate

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestAllEightNetworkProtocols(test *testing.T) {
	protocols := []schema.NetworkProtocol{schema.NetworkTCP, schema.NetworkUDP, schema.NetworkSCTP, schema.NetworkICMP, schema.NetworkICMPv6, schema.NetworkGRE, schema.NetworkESP, schema.NetworkAH}
	for _, protocol := range protocols {
		rule := outboundRule()
		rule.Protocols = []schema.NetworkProtocol{protocol}
		if err := Validate(networkCfg(rule)); err != nil {
			test.Fatalf("%s: %v", protocol, err)
		}
		for _, sourcePorts := range []bool{true, false} {
			if sourcePorts {
				rule.From.Filter.Ports = []int{1024, 65535}
			} else {
				rule.From.Filter.Ports = nil
				rule.To.Filter.Ports = []int{1, 443}
			}
			cfg := networkCfg(rule)
			switch protocol {
			case schema.NetworkTCP, schema.NetworkUDP, schema.NetworkSCTP:
				if err := Validate(cfg); err != nil {
					test.Fatalf("%s with ports: %v", protocol, err)
				}
			default:
				requireError(test, cfg, "ports are only supported")
			}
		}
	}
	rule := outboundRule()
	rule.Protocols = protocols
	if err := Validate(networkCfg(rule)); err != nil {
		test.Fatal(err)
	}
}

func TestEndpointFiltersAndProtocolCombinations(test *testing.T) {
	for _, filter := range []schema.NetworkPeerFilter{
		{IPv4CIDR: []string{"not-a-cidr"}}, {IPv4CIDR: []string{"::/0"}},
		{IPv6CIDR: []string{"0.0.0.0/0"}}, {IPv6CIDR: []string{"::ffff:192.0.2.0/120"}},
		{Ports: []int{0}}, {Ports: []int{-1}}, {Ports: []int{65536}},
	} {
		for _, endpoint := range []string{"From", "To"} {
			rule := outboundRule()
			rule.Protocols = []schema.NetworkProtocol{schema.NetworkTCP}
			if endpoint == "From" {
				rule.From.Filter = filter
			} else {
				rule.To.Filter = filter
			}
			requireError(test, networkCfg(rule), endpoint+".Filter")
		}
	}
	for _, protocols := range [][]schema.NetworkProtocol{nil, {"tcp"}, {""}, {"TCP", "ICMP"}, {"UDP", "GRE"}} {
		rule := outboundRule()
		rule.Protocols = protocols
		rule.To.Filter.Ports = []int{443}
		requireError(test, networkCfg(rule), "Protocols")
	}
	rule := outboundRule()
	rule.Protocols = []schema.NetworkProtocol{schema.NetworkTCP, schema.NetworkUDP, schema.NetworkSCTP}
	rule.From.Filter = schema.NetworkPeerFilter{IPv4CIDR: []string{"10.0.0.0/8"}, IPv6CIDR: []string{"fd00::/8"}, Ports: []int{1024}}
	rule.To.Filter = schema.NetworkPeerFilter{IPv4CIDR: []string{"0.0.0.0/0"}, IPv6CIDR: []string{"::/0"}, Ports: []int{443}}
	if err := Validate(networkCfg(rule)); err != nil {
		test.Fatal(err)
	}
	for _, protocol := range []schema.NetworkProtocol{schema.NetworkICMP, schema.NetworkICMPv6} {
		rule = outboundRule()
		rule.Protocols = []schema.NetworkProtocol{protocol}
		if protocol == schema.NetworkICMP {
			rule.From.Filter.IPv6CIDR = []string{"::/0"}
		} else {
			rule.To.Filter.IPv4CIDR = []string{"0.0.0.0/0"}
		}
		requireError(test, networkCfg(rule), "cannot match")
	}
}

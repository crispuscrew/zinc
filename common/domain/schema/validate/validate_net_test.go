package validate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func outboundRule() schema.NetworkRule {
	return schema.NetworkRule{
		From: schema.NetworkPeer{Type: schema.NetworkPeerSelf, Interface: "wan"},
		To:   schema.NetworkPeer{Type: schema.NetworkPeerInternet},
	}
}

func networkCfg(rules ...schema.NetworkRule) schema.AppConfig {
	cfg := baseCfg()
	cfg.NetworkMeta = schema.NetworkMeta{
		Interfaces: []schema.NetworkInterface{{ID: "wan"}}, RulesByPriority: rules,
		DNS: schema.DNSMeta{ResolversByPriority: []schema.DNSResolver{{Protocol: schema.DNSUDP, Endpoint: "1.1.1.1:53"}}},
	}
	return cfg
}

func TestAllPeerTypesForContainersAndVMs(test *testing.T) {
	for _, peerType := range []schema.NetworkPeerType{schema.NetworkPeerSelf, schema.NetworkPeerApp, schema.NetworkPeerAnyApp, schema.NetworkPeerHost, schema.NetworkPeerInternet, schema.NetworkPeerAny} {
		for _, appType := range []schema.Type{schema.ZincContainer, schema.ZincVirtualization} {
			rule := outboundRule()
			rule.To.Type = peerType
			if peerType == schema.NetworkPeerApp {
				rule.To.AppNameID = "other-app"
				rule.To.Interface = "private"
			}
			if peerType == schema.NetworkPeerHost {
				rule.To.Interface = "enp0s3.100"
			}
			cfg := networkCfg(rule)
			if appType == schema.ZincVirtualization {
				cfg = baseVM()
				cfg.NetworkMeta = networkCfg(rule).NetworkMeta
			}
			if err := Validate(cfg); err != nil {
				test.Fatalf("%s %s: %v", appType, peerType, err)
			}
			cfg.NetworkMeta.RulesByPriority[0].From, cfg.NetworkMeta.RulesByPriority[0].To = rule.To, rule.From
			if err := Validate(cfg); err != nil {
				test.Fatalf("incoming %s %s: %v", appType, peerType, err)
			}
		}
	}
}

func TestPeerDiscriminatorAndInterfaces(test *testing.T) {
	for _, peer := range []schema.NetworkPeer{
		{}, {Type: "Unknown"}, {Type: schema.NetworkPeerApp},
		{Type: schema.NetworkPeerApp, AppNameID: "../app"},
		{Type: schema.NetworkPeerSelf, AppNameID: "other"},
		{Type: schema.NetworkPeerHost, AppNameID: "other"},
		{Type: schema.NetworkPeerInternet, AppNameID: "other"},
		{Type: schema.NetworkPeerAnyApp, Interface: "wan"},
		{Type: schema.NetworkPeerAny, Interface: "wan"},
		{Type: schema.NetworkPeerInternet, Interface: "eth0"},
		{Type: schema.NetworkPeerSelf, Interface: "eth0"},
		{Type: schema.NetworkPeerApp, AppNameID: "other", Interface: "../wan"},
		{Type: schema.NetworkPeerHost, Interface: "eth0,other"},
		{Type: schema.NetworkPeerHost, Interface: strings.Repeat("a", 16)},
		{Type: schema.NetworkPeerHost, Interface: "."},
		{Type: schema.NetworkPeerHost, Interface: ".."},
	} {
		for _, endpoint := range []string{"From", "To"} {
			rule := outboundRule()
			if endpoint == "From" {
				rule.From = peer
			} else {
				rule.To = peer
			}
			requireError(test, networkCfg(rule), "RulesByPriority[0]."+endpoint)
		}
	}
}

func TestOrderedRulesAndNoImplicitReverseGrant(test *testing.T) {
	deny, allow := outboundRule(), outboundRule()
	deny.AllowAllExcept = true
	deny.To.Filter = schema.NetworkPeerFilter{IPv4CIDR: []string{"10.0.0.0/8"}}
	allow.To.Filter = schema.NetworkPeerFilter{IPv4CIDR: []string{"0.0.0.0/0"}, IPv6CIDR: []string{"::/0"}}
	cfg := networkCfg(deny, allow)
	if err := Validate(cfg); err != nil {
		test.Fatalf("one-way allowance needs no explicit reply rule: %v", err)
	}
	if !reflect.DeepEqual(cfg.NetworkMeta.RulesByPriority, []schema.NetworkRule{deny, allow}) {
		test.Fatal("validation changed ordered rules")
	}
	if warnings := Warnings(networkCfg(deny)); len(warnings) != 0 {
		test.Fatalf("deny rule must not be interpreted as allow-all: %v", warnings)
	}
	isolated := baseCfg()
	if err := Validate(isolated); err != nil || len(isolated.NetworkMeta.Interfaces) != 0 {
		test.Fatalf("isolated default must not acquire a NIC: %v", err)
	}
}

package validate

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestExplicitDNSProtocols(test *testing.T) {
	for _, protocol := range []schema.DNSProtocol{schema.DNSUDP, schema.DNSTCP, schema.DNSTLS, schema.DNSHTTPS, schema.DNSQUIC} {
		for _, endpoint := range []string{"1.1.1.1", "1.1.1.1:5353", "2606:4700:4700::1111", "[2606:4700:4700::1111]:853", "dns.example.com", "dns.example.com:443"} {
			resolver := schema.DNSResolver{Protocol: protocol, Endpoint: endpoint}
			if strings.HasPrefix(endpoint, "dns.") {
				resolver.BootstrapIPs = []string{"1.1.1.1", "2606:4700:4700::1111"}
			}
			if protocol == schema.DNSHTTPS {
				resolver.Path = "/dns-query"
			}
			cfg := networkCfg()
			cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{resolver}
			if err := Validate(cfg); err != nil {
				test.Fatalf("%s %s: %v", protocol, endpoint, err)
			}
		}
	}
}

func TestDNSRejectsImplicitResolutionAndEndpointInjection(test *testing.T) {
	for _, resolver := range []schema.DNSResolver{
		{Protocol: schema.DNSUDP},
		{Protocol: "DoH", Endpoint: "1.1.1.1"},
		{Protocol: schema.DNSTLS, Endpoint: "dns.example.com"},
		{Protocol: schema.DNSTCP, Endpoint: "https://dns.example.com/dns-query"},
		{Protocol: schema.DNSUDP, Endpoint: "1.1.1.1:0"},
		{Protocol: schema.DNSUDP, Endpoint: "1.1.1.1:65536"},
		{Protocol: schema.DNSUDP, Endpoint: "1.1.1.1:+53"},
		{Protocol: schema.DNSUDP, Endpoint: "1.1.1.1:domain"},
		{Protocol: schema.DNSUDP, Endpoint: "user@dns.example.com"},
		{Protocol: schema.DNSUDP, Endpoint: " 1.1.1.1"},
		{Protocol: schema.DNSUDP, Endpoint: "[fe80::1%eth0]:53"},
		{Protocol: schema.DNSUDP, Endpoint: "999.1.1.1", BootstrapIPs: []string{"1.1.1.1"}},
		{Protocol: schema.DNSUDP, Endpoint: "1.1.1.1", Path: "/dns-query"},
	} {
		cfg := networkCfg()
		cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{resolver}
		requireError(test, cfg, "DNS.ResolversByPriority[0]")
	}
	for _, bootstrap := range []string{"", "dns.example.com", "1.1.1.1:53", "1.1.1.1/32", "[::1]", "fe80::1%eth0", "1.1.1.1\n"} {
		cfg := networkCfg()
		cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSQUIC, Endpoint: "dns.example.com", BootstrapIPs: []string{bootstrap}}}
		requireError(test, cfg, "BootstrapIPs")
	}
	for _, path := range []string{"relative", "https://evil/dns", "//evil/dns", "/dns?redirect=evil", "/dns#fragment", "/dns\nquery", "/dns%00query", "/dns\\query", "/%zz"} {
		cfg := networkCfg()
		cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSHTTPS, Endpoint: "1.1.1.1", Path: path}}
		requireError(test, cfg, "Path")
	}
}

func TestDomainRulesRequireExplicitDNS(test *testing.T) {
	for _, domain := range []string{"", "https://example.com", "example.com:443", "example.com/path", "example.com.", "Example.com", " example.com", "*.example.com", strings.Repeat("a", 64) + ".com", strings.Repeat("a.", 127) + "a"} {
		rule := outboundRule()
		rule.Domains = []string{domain}
		requireError(test, networkCfg(rule), "Domains")
	}
	for _, deny := range []bool{false, true} {
		rule := outboundRule()
		rule.Domains = []string{"example.com", "api.example.com", "a-b.example"}
		rule.AllowAllExcept = deny
		cfg := networkCfg(rule)
		if err := Validate(cfg); err != nil {
			test.Fatal(err)
		}
		cfg.NetworkMeta.DNS.ResolversByPriority = nil
		requireError(test, cfg, "implicit host DNS")
	}
}

package network

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestUpstreamTransportAndPriority(t *testing.T) {
	meta := schema.DNSMeta{}
	for _, protocol := range []schema.DNSProtocol{schema.DNSUDP, schema.DNSTCP, schema.DNSTLS, schema.DNSHTTPS, schema.DNSQUIC} {
		meta.ResolversByPriority = append(meta.ResolversByPriority, schema.DNSResolver{Protocol: protocol, Endpoint: "dns.example.org", BootstrapIPs: []string{"1.1.1.1", "2606:4700::1111"}})
	}
	upstreams, err := Upstreams(meta)
	if err != nil {
		t.Fatal(err)
	}
	ports := []int{53, 53, 853, 443, 853}
	for index, upstream := range upstreams {
		if upstream.Protocol != meta.ResolversByPriority[index].Protocol || upstream.Host != "dns.example.org" || upstream.Port != ports[index] {
			t.Fatal("priority/authority/port lost")
		}
		wantUDP := index == 0 || index == 4
		if (upstream.Transport == schema.NetworkUDP) != wantUDP {
			t.Fatal("wrong transport")
		}
	}
	if upstreams[3].Path != "/dns-query" {
		t.Fatal("missing HTTPS default path")
	}
}

func TestUpstreamRejectsImplicitFallbackAndInvalidEndpoints(t *testing.T) {
	for _, resolver := range []schema.DNSResolver{
		{Protocol: schema.DNSTLS, Endpoint: "dns.example.org"},
		{Protocol: schema.DNSQUIC, Endpoint: "1.1.1.1:0"},
		{Protocol: schema.DNSUDP, Endpoint: "1.1.1.1", Path: "/dns-query"},
		{Protocol: schema.DNSHTTPS, Endpoint: "1.1.1.1", Path: "//other.example/dns"},
		{Protocol: schema.DNSHTTPS, Endpoint: "https://example.org/dns"},
		{Protocol: "BOGUS", Endpoint: "1.1.1.1"},
	} {
		if _, err := Upstreams(schema.DNSMeta{ResolversByPriority: []schema.DNSResolver{resolver}}); err == nil {
			t.Errorf("accepted %+v", resolver)
		}
	}
	upstreams, err := Upstreams(schema.DNSMeta{ResolversByPriority: []schema.DNSResolver{{Protocol: schema.DNSTLS, Endpoint: "[2606:4700::1111]:8853"}}})
	if err != nil || upstreams[0].Port != 8853 {
		t.Fatal("explicit IPv6 resolver port lost", err)
	}
}

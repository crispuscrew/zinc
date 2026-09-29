package schema

import (
	"strings"
	"testing"
)

func TestMigrateLosslessNetworkLists(t *testing.T) {
	config, _ := migratedConfig(t, `SchemaVersion: 4
NetworkMeta:
  NetworkLists:
    - IPv4CIDR: [192.0.2.0/24]
      IPv6CIDR: ['2001:db8::/32']
      Ports: [443]
      Domains: [example.org]
    - IPv4CIDR: [198.51.100.0/24]
      Blacklist: false
  DNSServers: [192.0.2.53, '2001:db8::53']
`)
	rules := config.NetworkMeta.RulesByPriority
	if len(rules) != 2 || rules[0].From.Type != NetworkPeerSelf || rules[0].To.Type != NetworkPeerInternet ||
		rules[0].To.Filter.Ports[0] != 443 || len(rules[0].Protocols) != 2 ||
		rules[0].Domains[0] != "example.org" || len(rules[0].From.Filter.Ports) != 0 ||
		rules[1].To.Filter.IPv4CIDR[0] != "198.51.100.0/24" {
		t.Fatalf("rule scope, filters or priority changed: %+v", rules)
	}
	resolvers := config.NetworkMeta.DNS.ResolversByPriority
	if len(resolvers) != 4 || resolvers[0].Protocol != DNSUDP || resolvers[1].Protocol != DNSTCP ||
		resolvers[2].Endpoint != "[2001:db8::53]:53" {
		t.Fatalf("DNS priority/fallback lost: %+v", resolvers)
	}
	_, output := migratedConfig(t, "NetworkMeta: {NetworkLists: null, DNSServers: null}\n")
	if !strings.Contains(output, "RulesByPriority: null") || !strings.Contains(output, "ResolversByPriority: null") {
		t.Fatalf("network nulls lost: %s", output)
	}
}

func TestMigrateRefusesAmbiguousNetwork(t *testing.T) {
	for _, field := range []string{
		"Via: true", "Forward: true", "ForwardPorts: [443]", "GatewayV4: 192.0.2.1",
		"GatewayV6: '2001:db8::1'", "Host: true", "AppName: proxy", "Interface: eth0",
		"Ingress: true", "Blacklist: true", "Unknown: false",
	} {
		input := "NetworkMeta:\n  NetworkLists:\n    - IPv4CIDR: [192.0.2.0/24]\n      " + field + "\n"
		if _, err := Migrate([]byte(input)); err == nil || !strings.Contains(err.Error(), strings.Split(field, ":")[0]) {
			t.Fatalf("ambiguous %s migration: %v", field, err)
		}
	}
	for _, input := range []string{
		"NetworkMeta: {NetworkLists: [], RulesByPriority: []}\n",
		"NetworkMeta: {DNSServers: [], DNS: {ResolversByPriority: []}}\n",
		"NetworkMeta: {NetworkLists: [{}]}\n",
		"NetworkMeta: {DNSServers: [resolver.example.org]}\n",
	} {
		if _, err := Migrate([]byte(input)); err == nil {
			t.Fatalf("ambiguous network accepted: %s", input)
		}
	}
}

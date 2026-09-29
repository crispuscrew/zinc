package schema

import (
	"fmt"
	"net"

	"gopkg.in/yaml.v3"
)

func (change *migration) migrateDNS() error {
	servers := change.value("NetworkMeta.DNSServers")
	if servers == nil {
		return nil
	}
	if servers.Tag == "!!null" {
		return change.move("NetworkMeta.DNSServers", "NetworkMeta.DNS.ResolversByPriority")
	}
	if servers.Kind != yaml.SequenceNode {
		return fmt.Errorf("NetworkMeta.DNSServers must be a list of IP addresses")
	}
	resolvers := make([]DNSResolver, 0, len(servers.Content)*2)
	for _, server := range servers.Content {
		if server.Tag != "!!str" || net.ParseIP(server.Value) == nil {
			return fmt.Errorf("NetworkMeta.DNSServers: %q is not an IP address; specify DNS.ResolversByPriority explicitly", server.Value)
		}
		for _, protocol := range []DNSProtocol{DNSUDP, DNSTCP} {
			resolvers = append(resolvers, DNSResolver{Protocol: protocol, Endpoint: net.JoinHostPort(server.Value, "53")})
		}
	}
	if err := change.putValue("NetworkMeta.DNS.ResolversByPriority", resolvers); err != nil {
		return fmt.Errorf("NetworkMeta.DNSServers: %w", err)
	}
	change.remove("NetworkMeta.DNSServers")
	return nil
}

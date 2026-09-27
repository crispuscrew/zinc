package compose

import (
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func exportNetwork(network schema.NetworkMeta, note func(string, ...any)) (ports, expose StringList) {
	// A later allow cannot be exported independently of an earlier exclusion.
	for _, rule := range network.RulesByPriority {
		if rule.AllowAllExcept {
			note("RulesByPriority contains exclusions: ordered allow/deny behavior is not representable, so no ports were published")
			return
		}
	}
	for index, rule := range network.RulesByPriority {
		if rule.To.Type != schema.NetworkPeerSelf {
			note("RulesByPriority[%d] peer connection is not represented", index)
			continue
		}
		if len(rule.Domains) > 0 || rule.From.Interface != "" || len(rule.From.Filter.IPv4CIDR)+len(rule.From.Filter.IPv6CIDR)+len(rule.From.Filter.Ports)+len(rule.To.Filter.IPv4CIDR)+len(rule.To.Filter.IPv6CIDR) > 0 {
			note("RulesByPriority[%d] address/interface/domain restriction cannot be represented; its ports were not published", index)
			continue
		}
		if len(rule.Protocols) == 0 {
			note("RulesByPriority[%d] has unspecified transports; ports were not guessed", index)
			continue
		}
		for _, protocol := range rule.Protocols {
			if protocol != schema.NetworkTCP && protocol != schema.NetworkUDP && protocol != schema.NetworkSCTP {
				note("RulesByPriority[%d] protocol %s has no Compose port representation", index, protocol)
				continue
			}
			for _, port := range rule.To.Filter.Ports {
				text := strconv.Itoa(port)
				suffix := "/" + strings.ToLower(string(protocol))
				switch rule.From.Type {
				case schema.NetworkPeerHost:
					ports = append(ports, "127.0.0.1:"+text+":"+text+suffix)
				case schema.NetworkPeerAny:
					ports = append(ports, text+":"+text+suffix)
				case schema.NetworkPeerAnyApp:
					expose = append(expose, text+suffix)
				default:
					note("RulesByPriority[%d] source %s cannot be represented; port %d was not published", index, rule.From.Type, port)
				}
			}
		}
	}
	return
}

func exportDNS(dns schema.DNSMeta, note func(string, ...any)) StringList {
	var servers StringList
	for index, resolver := range dns.ResolversByPriority {
		host, port, err := net.SplitHostPort(resolver.Endpoint)
		if err != nil {
			host, port = resolver.Endpoint, "53"
		}
		if (resolver.Protocol != schema.DNSUDP && resolver.Protocol != schema.DNSTCP) || port != "53" || net.ParseIP(host) == nil || resolver.Path != "" || len(resolver.BootstrapIPs) > 0 {
			note("DNS.ResolversByPriority[%d] is not representable by Compose dns IPs", index)
			continue
		}
		if !slices.Contains(servers, host) {
			servers = append(servers, host)
		}
	}
	if len(servers) > 0 {
		note("DNS resolver IPs are represented only as settings; transport, fallback priority and resolver restrictions are not enforced by Compose")
	}
	return servers
}

package validate

import (
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Rules are first-match, default deny. AllowAllExcept denies that match, never
// changes the default. Stateful replies and both app endpoints' permission are
// enforced at runtime; neither requires a reverse rule in this app's config.
// Internet is public-only: CIDRs narrow that scope, never extend it to private
// addresses. Resolving App interface IDs and intersecting the remote app's
// permission are launch-time checks, since this validator has only one config.
func checkNetwork(network schema.NetworkMeta, add addFunc) {
	interfaces := checkInterfaces(network.Interfaces, add)
	checkDNS(network.DNS, add)
	for index, rule := range network.RulesByPriority {
		field := fmt.Sprintf("NetworkMeta.RulesByPriority[%d]", index)
		checkPeer(field+".From", rule.From, interfaces, add)
		checkPeer(field+".To", rule.To, interfaces, add)
		checkProtocols(field, rule, add)
		for position, domain := range rule.Domains {
			if !validDomain(domain) {
				add("%s.Domains[%d] %q: must be a plain lowercase hostname with labels of 1-63 characters, at most 253 total", field, position, domain)
			}
		}
		if len(rule.Domains) > 0 && len(network.DNS.ResolversByPriority) == 0 {
			add("%s.Domains: requires NetworkMeta.DNS.ResolversByPriority; implicit host DNS is not permitted", field)
		}
	}
}

func checkPeer(field string, peer schema.NetworkPeer, interfaces map[string]bool, add addFunc) {
	switch peer.Type {
	case schema.NetworkPeerSelf, schema.NetworkPeerApp, schema.NetworkPeerAnyApp,
		schema.NetworkPeerHost, schema.NetworkPeerInternet, schema.NetworkPeerAny:
	default:
		add("%s.Type %q: must be Self, App, AnyApp, Host, Internet or Any", field, peer.Type)
	}
	if peer.Type == schema.NetworkPeerApp {
		if !nameRE.MatchString(peer.AppNameID) {
			add("%s.AppNameID %q: App requires a valid app ID (lowercase [a-z0-9._-], starting alphanumeric)", field, peer.AppNameID)
		}
	} else if peer.AppNameID != "" {
		add("%s.AppNameID: only allowed with Type App", field)
	}
	if peer.Interface != "" {
		switch peer.Type {
		case schema.NetworkPeerHost:
			if !validHostInterface(peer.Interface) {
				add("%s.Interface %q: must be a real host interface name, 1-15 bytes of [A-Za-z0-9._-], not '.' or '..'", field, peer.Interface)
			}
		case schema.NetworkPeerSelf:
			if !interfaces[peer.Interface] {
				add("%s.Interface %q: must reference an ID in NetworkMeta.Interfaces", field, peer.Interface)
			}
		case schema.NetworkPeerApp:
			if !nameRE.MatchString(peer.Interface) {
				add("%s.Interface %q: must be a Zinc interface ID (lowercase [a-z0-9._-], starting alphanumeric)", field, peer.Interface)
			}
		default:
			add("%s.Interface: only allowed with Type Self, App or Host", field)
		}
	}
	for _, cidr := range peer.Filter.IPv4CIDR {
		if !validCIDR(cidr, false) {
			add("%s.Filter.IPv4CIDR %q: not a valid IPv4 CIDR", field, cidr)
		}
	}
	for _, cidr := range peer.Filter.IPv6CIDR {
		if !validCIDR(cidr, true) {
			add("%s.Filter.IPv6CIDR %q: not a valid IPv6 CIDR", field, cidr)
		}
	}
	for _, port := range peer.Filter.Ports {
		if port < 1 || port > 65535 {
			add("%s.Filter.Ports %d: out of range 1-65535", field, port)
		}
	}
}

// Empty Protocols means any supported protocol, but a port needs an explicit
// transport so it cannot accidentally narrow or ignore a non-port protocol.
func checkProtocols(field string, rule schema.NetworkRule, add addFunc) {
	ports := len(rule.From.Filter.Ports) > 0 || len(rule.To.Filter.Ports) > 0
	if ports && len(rule.Protocols) == 0 {
		add("%s.Protocols: ports require explicit TCP, UDP or SCTP protocols", field)
	}
	for index, protocol := range rule.Protocols {
		switch protocol {
		case schema.NetworkTCP, schema.NetworkUDP, schema.NetworkSCTP:
		case schema.NetworkICMP, schema.NetworkICMPv6, schema.NetworkGRE, schema.NetworkESP, schema.NetworkAH:
			if ports {
				add("%s.Protocols[%d] %q: ports are only supported with TCP, UDP or SCTP", field, index, protocol)
			}
		default:
			add("%s.Protocols[%d] %q: unknown protocol (TCP, UDP, ICMP, ICMPv6, SCTP, GRE, ESP, AH)", field, index, protocol)
		}
		if protocol == schema.NetworkICMP && ipv6Only(rule.From.Filter, rule.To.Filter) {
			add("%s.Protocols[%d]: ICMP cannot match an IPv6-only endpoint; use ICMPv6", field, index)
		}
		if protocol == schema.NetworkICMPv6 && ipv4Only(rule.From.Filter, rule.To.Filter) {
			add("%s.Protocols[%d]: ICMPv6 cannot match an IPv4-only endpoint; use ICMP", field, index)
		}
	}
}

func ipv6Only(filters ...schema.NetworkPeerFilter) bool {
	for _, filter := range filters {
		if len(filter.IPv6CIDR) > 0 && len(filter.IPv4CIDR) == 0 {
			return true
		}
	}
	return false
}

func ipv4Only(filters ...schema.NetworkPeerFilter) bool {
	for _, filter := range filters {
		if len(filter.IPv4CIDR) > 0 && len(filter.IPv6CIDR) == 0 {
			return true
		}
	}
	return false
}

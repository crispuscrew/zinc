package network

import (
	"fmt"
	"net/netip"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

var ProtocolNumbers = map[schema.NetworkProtocol]int{
	schema.NetworkTCP: 6, schema.NetworkUDP: 17, schema.NetworkICMP: 1,
	schema.NetworkICMPv6: 58, schema.NetworkSCTP: 132, schema.NetworkGRE: 47,
	schema.NetworkESP: 50, schema.NetworkAH: 51,
}

func policy(resolved Resolved, owner string, meta schema.NetworkMeta) error {
	for index, rule := range meta.RulesByPriority {
		for _, peer := range []schema.NetworkPeer{rule.From, rule.To} {
			if err := validatePeer(resolved, owner, peer); err != nil {
				return fmt.Errorf("%s rule[%d]: %w", owner, index, err)
			}
		}
		hasPorts := len(rule.From.Filter.Ports)+len(rule.To.Filter.Ports) > 0
		if hasPorts && len(rule.Protocols) == 0 {
			return fmt.Errorf("%s rule[%d]: ports require explicit transports", owner, index)
		}
		for _, protocol := range rule.Protocols {
			if _, exists := ProtocolNumbers[protocol]; !exists {
				return fmt.Errorf("unknown protocol %q", protocol)
			}
			if hasPorts && protocol != schema.NetworkTCP && protocol != schema.NetworkUDP && protocol != schema.NetworkSCTP {
				return fmt.Errorf("ports cannot match %s", protocol)
			}
		}
		if len(rule.Domains) > 0 {
			if len(meta.DNS.ResolversByPriority) == 0 {
				return fmt.Errorf("%s rule[%d]: domains require approved DNS", owner, index)
			}
			addresses := resolved.Domains[owner][index]
			if err := validateAddresses(addresses); err != nil {
				return fmt.Errorf("%s rule[%d]: approved domain resolution required: %w", owner, index, err)
			}
		}
	}
	return nil
}

func validatePeer(resolved Resolved, owner string, peer schema.NetworkPeer) error {
	if peer.Type != schema.NetworkPeerApp && peer.AppNameID != "" {
		return fmt.Errorf("AppNameID requires App peer")
	}
	var endpoints []Attachment
	switch peer.Type {
	case schema.NetworkPeerSelf:
		endpoints = resolved.Endpoints(owner)
	case schema.NetworkPeerApp:
		endpoints = resolved.Endpoints(peer.AppNameID)
		if len(endpoints) == 0 {
			return fmt.Errorf("unknown provisioned app %q", peer.AppNameID)
		}
	case schema.NetworkPeerHost:
		if len(resolved.Topology.Hosts) == 0 {
			return fmt.Errorf("Host peer requires authoritative host addresses")
		}
	case schema.NetworkPeerInternet:
		if len(resolved.Topology.ExternalInterfaces) == 0 || len(resolved.Topology.Hosts) == 0 {
			return fmt.Errorf("Internet requires external interfaces and host inventory")
		}
	case schema.NetworkPeerAnyApp, schema.NetworkPeerAny:
	default:
		return fmt.Errorf("unknown peer type %q", peer.Type)
	}
	if peer.Interface != "" {
		found := false
		for _, endpoint := range endpoints {
			found = found || endpoint.InterfaceID == peer.Interface
		}
		if peer.Type == schema.NetworkPeerHost {
			for _, host := range resolved.Topology.Hosts {
				found = found || host.Interface == peer.Interface
			}
		}
		if !found {
			return fmt.Errorf("unresolved interface %q for %s", peer.Interface, peer.Type)
		}
	}
	for family, prefixes := range [][]string{peer.Filter.IPv4CIDR, peer.Filter.IPv6CIDR} {
		for _, value := range prefixes {
			prefix, err := netip.ParsePrefix(value)
			if err != nil || prefix.Addr().Is4In6() || prefix.Addr().Is4() != (family == 0) {
				return fmt.Errorf("invalid family CIDR %q", value)
			}
		}
	}
	for _, port := range peer.Filter.Ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid port %d", port)
		}
	}
	return nil
}

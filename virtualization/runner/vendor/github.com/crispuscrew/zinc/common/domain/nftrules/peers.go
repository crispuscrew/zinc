package nftrules

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func deviceKeyword(hook, side string) string {
	if hook == "input" || hook == "forward" && side == "s" {
		return "iifname"
	}
	return "oifname"
}

func peerMatches(resolved network.Resolved, owner string, peer schema.NetworkPeer, family, side, hook string) []string {
	var entries []network.Attachment
	switch peer.Type {
	case schema.NetworkPeerSelf:
		entries = resolved.Endpoints(owner)
	case schema.NetworkPeerApp:
		entries = resolved.Endpoints(peer.AppNameID)
	case schema.NetworkPeerAnyApp:
		entries = append(entries, resolved.Topology.Interfaces...)
		for _, app := range resolved.Topology.Peers {
			entries = append(entries, app.Interfaces...)
		}
	case schema.NetworkPeerHost:
		for _, host := range resolved.Topology.Hosts {
			entries = append(entries, network.Attachment{InterfaceID: host.Interface, Device: host.Device, Addresses: host.Addresses})
		}
	case schema.NetworkPeerAny:
		return []string{""}
	case schema.NetworkPeerInternet:
		var result []string
		for _, device := range resolved.Topology.ExternalInterfaces {
			result = append(result, fmt.Sprintf("%s %q %s %saddr != @nonpublic_%s", deviceKeyword(hook, side), device, family, side, family))
		}
		return result
	}
	var result []string
	for _, entry := range entries {
		if peer.Interface != "" && peer.Interface != entry.InterfaceID {
			continue
		}
		match := addressMatch(entry.Addresses, family, side)
		if match != "" {
			result = append(result, fmt.Sprintf("%s %q %s", deviceKeyword(hook, side), entry.Device, match))
		}
	}
	return result
}

func filterMatch(filter schema.NetworkPeerFilter, family, side string) (string, bool) {
	values := filter.IPv4CIDR
	if family == "ip6" {
		values = filter.IPv6CIDR
	}
	if len(filter.IPv4CIDR)+len(filter.IPv6CIDR) == 0 {
		return "", true
	}
	match := addressMatch(values, family, side)
	return match, match != ""
}

func portMatch(ports []int, protocol schema.NetworkProtocol, side string) string {
	if len(ports) == 0 {
		return ""
	}
	var values []string
	for _, port := range ports {
		values = append(values, fmt.Sprint(port))
	}
	return fmt.Sprintf("%s %sport { %s }", strings.ToLower(string(protocol)), side, strings.Join(values, ", "))
}

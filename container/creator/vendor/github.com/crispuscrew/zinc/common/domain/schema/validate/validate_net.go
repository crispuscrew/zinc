package validate

import (
	"net"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// checkNetworkList validates one entry (list order = priority, first wins). A list is
// directional: Ingress=false (default) is an egress rule (Ports = destination ports the
// app may reach); Ingress=true publishes the app's own listening Ports inbound. Scope:
// Host=true = host netns (egress) or a host-interface bind (ingress LAN); Host=false +
// empty AppName = this app (self); Host=false + AppName = a sibling.
func checkNetworkList(index int, netList schema.NetworkList, add addFunc) {
	for _, cidr := range netList.IPv4CIDR {
		if !validCIDR(cidr, false) {
			add("NetworkLists[%d].IPv4CIDR %q: not a valid IPv4 CIDR", index, cidr)
		}
	}
	for _, cidr := range netList.IPv6CIDR {
		if !validCIDR(cidr, true) {
			add("NetworkLists[%d].IPv6CIDR %q: not a valid IPv6 CIDR", index, cidr)
		}
	}
	for _, port := range netList.Ports {
		if port < 1 || port > 65535 {
			add("NetworkLists[%d].Ports %d: out of range 1-65535", index, port)
		}
	}
	if iface := netList.Interface; iface != "" && !ifaceRE.MatchString(iface) {
		add("NetworkLists[%d].Interface %q: only [A-Za-z0-9._-] allowed (no commas or spaces)", index, iface)
	}

	// Egress: a port carve-out attaches to a destination CIDR (nft `daddr … dport …`);
	// ports with no CIDR emit nothing and silently revert to the chain's default policy —
	// so a blacklist [53,853] with no CIDR silently keeps DNS open. Reject it: name the
	// destinations (0.0.0.0/0 and/or ::/0 for "everywhere"), or drop the ports. An ingress
	// list needs no CIDR — its CIDRs are a source allowlist and empty means "any source".
	if !netList.Ingress && len(netList.Ports) > 0 &&
		len(netList.IPv4CIDR) == 0 && len(netList.IPv6CIDR) == 0 {
		add("NetworkLists[%d].Ports %s: set without any IPv4CIDR/IPv6CIDR; an egress port rule needs destination CIDRs (use 0.0.0.0/0 and/or ::/0 for all destinations)", index, joinPorts(netList.Ports))
	}

	self := !netList.Host && strings.TrimSpace(netList.AppName) == ""
	if !netList.Host && netList.AppName != "" && !nameRE.MatchString(netList.AppName) {
		add("NetworkLists[%d].AppName %q: invalid app name; allowed [a-z0-9._-], must start alphanumeric", index, netList.AppName)
	}

	checkGateway(index, netList, self, add)
}

// checkGateway validates routing gateways and gates the multi-homing they imply. A
// gateway is one next-hop per family (not a range), so it needs a reachable link
// (host/sibling) and same-family destination CIDRs to carry.
func checkGateway(index int, netList schema.NetworkList, self bool, add addFunc) {
	hasV4, hasV6 := netList.GatewayV4 != "", netList.GatewayV6 != ""
	if !hasV4 && !hasV6 {
		return
	}

	if hasV4 {
		if ip := net.ParseIP(netList.GatewayV4); ip == nil || ip.To4() == nil {
			add("NetworkLists[%d].GatewayV4 %q: not a valid IPv4 address", index, netList.GatewayV4)
		} else if len(netList.IPv4CIDR) == 0 {
			add("NetworkLists[%d].GatewayV4: set but no IPv4CIDR destinations to route through it", index)
		}
	}
	if hasV6 {
		if ip := net.ParseIP(netList.GatewayV6); ip == nil || ip.To4() != nil {
			add("NetworkLists[%d].GatewayV6 %q: not a valid IPv6 address", index, netList.GatewayV6)
		} else if len(netList.IPv6CIDR) == 0 {
			add("NetworkLists[%d].GatewayV6: set but no IPv6CIDR destinations to route through it", index)
		}
	}
	if self {
		// Own netns has no next-hop to route through — a gateway needs host/sibling.
		add("NetworkLists[%d]: a gateway needs a host or sibling AppName link, not the app's own netns", index)
	}

	// Multi-homing (extra interface + ip-rule/ip-route policy routing) isn't
	// implemented yet; the fields are schema-legal but a config using them is rejected.
	add("NetworkLists[%d]: routing through a gateway (multi-homing) is not supported in this build yet", index)
}

// joinPorts formats a port list for a message, e.g. []int{53, 853} -> "53, 853".
func joinPorts(ports []int) string {
	parts := make([]string, len(ports))
	for idx, port := range ports {
		parts[idx] = strconv.Itoa(port)
	}
	return strings.Join(parts, ", ")
}

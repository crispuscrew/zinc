package nftrules

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/network"
)

// Conservative public-unicast boundary, supplemented with exact host/app IPs.
var nonPublic4 = []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/3"}
var nonPublic6 = []string{"::/3", "4000::/2", "8000::/1", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"}

func writeSets(result *strings.Builder, resolved network.Resolved) {
	addresses := append([]string{}, nonPublic4...)
	addresses = append(addresses, nonPublic6...)
	for _, entry := range resolved.Topology.Interfaces {
		addresses = append(addresses, entry.Addresses...)
	}
	for _, peer := range resolved.Topology.Peers {
		for _, entry := range peer.Interfaces {
			addresses = append(addresses, entry.Addresses...)
		}
	}
	for _, host := range resolved.Topology.Hosts {
		addresses = append(addresses, host.Addresses...)
	}
	for _, family := range []string{"ip", "ip6"} {
		kind := "ipv4_addr"
		if family == "ip6" {
			kind = "ipv6_addr"
		}
		fmt.Fprintf(result, " set nonpublic_%s { type %s; flags interval; auto-merge; elements = { %s }; }\n", family, kind, strings.Join(familyAddresses(addresses, family), ", "))
	}
}

func familyAddresses(values []string, family string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, parseErr := netip.ParseAddr(value)
			if parseErr != nil {
				continue
			} // inputs were validated by RenderResolved
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		if prefix.Addr().Is4() == (family == "ip") {
			seen[prefix.Masked().String()] = true
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func addressMatch(values []string, family, side string) string {
	addresses := familyAddresses(values, family)
	if len(addresses) == 0 {
		return ""
	}
	return fmt.Sprintf("%s %saddr { %s }", family, side, strings.Join(addresses, ", "))
}

func devices(entries []network.Attachment) string {
	var names []string
	for _, entry := range entries {
		names = append(names, fmt.Sprintf("%q", entry.Device))
	}
	return "{ " + strings.Join(names, ", ") + " }"
}

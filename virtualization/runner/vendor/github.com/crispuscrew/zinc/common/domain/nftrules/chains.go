package nftrules

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/network"
)

func writeChain(result *strings.Builder, resolved network.Resolved, hook string, active bool, mark uint32) {
	fmt.Fprintf(result, " chain %s {\n  type filter hook %s priority 0; policy drop;\n", hook, hook)
	if active {
		writeGuards(result, resolved, hook)
		result.WriteString("  ct state invalid counter drop comment \"invalid\"\n")
		fmt.Fprintf(result, "  ct state established,related ct mark %d counter accept comment \"authorized replies\"\n", mark)
		fmt.Fprintf(result, "  jump owner_%s\n", hook)
		for index, peer := range resolved.Topology.Peers {
			for _, family := range []string{"ip", "ip6"} {
				for _, side := range []string{"s", "d"} {
					var addresses []string
					for _, entry := range peer.Interfaces {
						addresses = append(addresses, entry.Addresses...)
					}
					if match := addressMatch(addresses, family, side); match != "" {
						fmt.Fprintf(result, "  %s jump peer_%d_%s\n", match, index, hook)
					}
				}
			}
		}
		fmt.Fprintf(result, "  ct mark set %d counter accept comment \"both endpoints authorized\"\n", mark)
	}
	result.WriteString("  counter drop comment \"default\"\n }\n")
}

func writeGuards(result *strings.Builder, resolved network.Resolved, hook string) {
	local := resolved.Topology.Interfaces
	switch hook {
	case "input":
		fmt.Fprintf(result, "  iifname != %s drop\n", devices(local))
	case "output":
		fmt.Fprintf(result, "  oifname != %s drop\n", devices(local))
	case "forward":
		fmt.Fprintf(result, "  iifname != %s oifname != %s drop\n", devices(local), devices(local))
	}
	for _, entry := range local {
		for _, family := range []string{"ip", "ip6"} {
			if match := addressMatch(entry.Addresses, family, "s"); match != "" && hook != "output" {
				if hook == "input" {
					fmt.Fprintf(result, "  %s counter drop comment \"self source spoof\"\n", match)
				} else {
					fmt.Fprintf(result, "  %s iifname != %q counter drop comment \"self route spoof\"\n", match, entry.Device)
				}
			}
			if hook != "input" {
				guardAddress(result, entry, family, "s", deviceKeyword(hook, "s"))
			}
			if hook != "output" {
				guardAddress(result, entry, family, "d", deviceKeyword(hook, "d"))
			}
		}
	}
	for _, host := range resolved.Topology.Hosts {
		for _, family := range []string{"ip", "ip6"} {
			if match := addressMatch(host.Addresses, family, "s"); match != "" && hook != "output" {
				fmt.Fprintf(result, "  %s iifname != %q counter drop comment \"host route spoof\"\n", match, host.Device)
			}
		}
	}
	// A peer's claimed source address must arrive on its provisioned route.
	for _, peer := range resolved.Topology.Peers {
		for _, entry := range peer.Interfaces {
			for _, family := range []string{"ip", "ip6"} {
				if match := addressMatch(entry.Addresses, family, "s"); match != "" && hook != "output" {
					fmt.Fprintf(result, "  %s iifname != %q counter drop comment \"peer spoof\"\n", match, entry.Device)
				}
			}
		}
	}
}

func guardAddress(result *strings.Builder, entry network.Attachment, family, side, device string) {
	version := "ipv4"
	if family == "ip6" {
		version = "ipv6"
	}
	addresses := familyAddresses(entry.Addresses, family)
	if len(addresses) == 0 {
		fmt.Fprintf(result, "  %s %q meta nfproto %s counter drop comment \"unassigned family\"\n", device, entry.Device, version)
		return
	}
	fmt.Fprintf(result, "  %s %q %s %saddr != { %s } counter drop comment \"endpoint spoof\"\n", device, entry.Device, family, side, strings.Join(addresses, ", "))
}

func writeLinkGuards(result *strings.Builder, resolved network.Resolved) {
	result.WriteString("table netdev zinc_link\ndelete table netdev zinc_link\ntable netdev zinc_link {\n")
	for index, entry := range resolved.Topology.Interfaces {
		fmt.Fprintf(result, " chain tap_%d {\n  type filter hook ingress device %q priority -500; policy drop;\n", index, entry.Device)
		fmt.Fprintf(result, "  ether saddr != %s counter drop comment \"MAC spoof\"\n", entry.MAC)
		if addresses := familyAddresses(entry.Addresses, "ip"); len(addresses) > 0 {
			fmt.Fprintf(result, "  ether type arp arp saddr ether %s arp saddr ip { %s } accept\n", entry.MAC, strings.Join(addresses, ", "))
		}
		result.WriteString("  ether type { ip, ip6 } accept\n  counter drop comment \"unprovisioned L2\"\n }\n")
	}
	result.WriteString("}\n")
}

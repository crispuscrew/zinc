package nftrules

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func writePolicy(result *strings.Builder, resolved network.Resolved, owner string, meta schema.NetworkMeta, hook, chain string) {
	fmt.Fprintf(result, " chain %s {\n", chain)
	for index, rule := range meta.RulesByPriority {
		protocols := rule.Protocols
		if len(protocols) == 0 {
			protocols = []schema.NetworkProtocol{schema.NetworkTCP, schema.NetworkUDP, schema.NetworkICMP,
				schema.NetworkICMPv6, schema.NetworkSCTP, schema.NetworkGRE, schema.NetworkESP, schema.NetworkAH}
		}
		for _, family := range []string{"ip", "ip6"} {
			fromFilter, fromOK := filterMatch(rule.From.Filter, family, "s")
			toFilter, toOK := filterMatch(rule.To.Filter, family, "d")
			if !fromOK || !toOK {
				continue
			}
			domain := ""
			if len(rule.Domains) > 0 {
				domain = addressMatch(resolved.Domains[owner][index], family, "d")
				if domain == "" {
					continue
				}
			}
			for _, protocol := range protocols {
				if protocol == schema.NetworkICMP && family != "ip" || protocol == schema.NetworkICMPv6 && family != "ip6" {
					continue
				}
				transport := ""
				if protocol != "" {
					transport = fmt.Sprintf("meta l4proto %d", network.ProtocolNumbers[protocol])
				}
				for _, from := range peerMatches(resolved, owner, rule.From, family, "s", hook) {
					for _, to := range peerMatches(resolved, owner, rule.To, family, "d", hook) {
						verdict := "return" // caller still has to check the other endpoint
						if rule.AllowAllExcept {
							verdict = "drop"
						}
						version := "ipv4"
						if family == "ip6" {
							version = "ipv6"
						}
						parts := []string{"meta nfproto " + version, from, to, fromFilter, toFilter, domain, transport,
							portMatch(rule.From.Filter.Ports, protocol, "s"), portMatch(rule.To.Filter.Ports, protocol, "d"),
							counted(verdict, fmt.Sprintf("%s rule[%d] %s %s", owner, index, family, protocol))}
						fmt.Fprintf(result, "  %s\n", strings.Join(parts, " "))
					}
				}
			}
		}
	}
	fmt.Fprintf(result, "  %s\n }\n", counted("drop", owner+" default"))
}

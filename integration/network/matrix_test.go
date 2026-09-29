package network_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestLiveContainerPackets(test *testing.T)      { transportMatrix(test, false) }
func TestLiveForwardedGuestPackets(test *testing.T) { transportMatrix(test, true) }

func transportMatrix(test *testing.T, routed bool) {
	for _, protocol := range []string{"tcp", "udp"} {
		for _, ipv6 := range []bool{false, true} {
			for _, inbound := range []bool{false, true} {
				test.Run(fmt.Sprintf("%s/ipv6=%t/inbound=%t", protocol, ipv6, inbound), func(test *testing.T) {
					lab := newLab(test, routed)
					source, destination, clientNS, serverNS := lab.client4, lab.server4, lab.client, lab.server
					if ipv6 {
						source, destination = lab.client6, lab.server6
					}
					if inbound {
						source, destination, clientNS, serverNS = destination, source, lab.server, lab.client
					}
					startServer(test, serverNS, protocol, destination, 8080)
					requireControl(test, clientNS, protocol, source, destination)
					policyCases(test, lab, protocol, source, destination, clientNS, inbound)
				})
			}
		}
	}
}

func policyCases(test *testing.T, lab laboratory, protocol, source, destination, clientNS string, inbound bool) {
	cases := []struct {
		name, counter, untouched string
		allowed                  bool
	}{
		{"default-deny", "client default", "", false},
		{"deny-only-match", "client rule[0]", "", false},
		{"deny-only-nonmatch", "client default", "client rule[0]", false},
		{"deny-before-allow", "client rule[0]", "client rule[1]", false},
		{"allow-before-deny", "client rule[0]", "client rule[1]", true},
		{"exact-source-destination-ports", "client rule[0]", "", true},
		{"wrong-source-port", "client default", "client rule[0]", false},
		{"wrong-destination-port", "client default", "client rule[0]", false},
		{"wrong-source-cidr", "client default", "client rule[0]", false},
		{"wrong-destination-cidr", "client default", "client rule[0]", false},
		{"peer-default-deny", "server default", "both endpoints authorized", false},
		{"peer-explicit-deny", "server rule[0]", "both endpoints authorized", false},
	}
	for _, example := range cases {
		test.Run(example.name, func(test *testing.T) {
			port := nextPort()
			allow := matchingRule(protocol, source, destination, port, inbound, false)
			deny := allow
			deny.AllowAllExcept = true
			rules := []schema.NetworkRule{allow}
			peerRules := []schema.NetworkRule{matchingRule(protocol, source, destination, port, inbound, true)}
			switch example.name {
			case "default-deny":
				rules = nil
			case "deny-only-match":
				rules = []schema.NetworkRule{deny}
			case "deny-only-nonmatch":
				deny.To.Filter.Ports = []int{8081}
				rules = []schema.NetworkRule{deny}
			case "deny-before-allow":
				rules = []schema.NetworkRule{deny, allow}
			case "allow-before-deny":
				rules = []schema.NetworkRule{allow, deny}
			case "wrong-source-port":
				rules[0].From.Filter.Ports = []int{port + 1}
			case "wrong-destination-port":
				rules[0].To.Filter.Ports = []int{8081}
			case "wrong-source-cidr":
				rules[0].From.Filter.IPv4CIDR, rules[0].From.Filter.IPv6CIDR = []string{"203.0.113.77/32"}, []string{"2001:db8:ff::77/128"}
			case "wrong-destination-cidr":
				rules[0].To.Filter.IPv4CIDR, rules[0].To.Filter.IPv6CIDR = []string{"203.0.113.77/32"}, []string{"2001:db8:ff::77/128"}
			case "peer-default-deny":
				peerRules = nil
			case "peer-explicit-deny":
				peerRules[0].AllowAllExcept = true
			}
			lab.apply(test, rules, peerRules)
			result := runProbe(test, clientNS, protocol, source, destination, port, 8080)
			verifyProbe(test, result, example.allowed)
			values := counters(test, lab)
			hits := packets(values, example.counter)
			if hits == 0 {
				test.Fatalf("expected actual packets in %q: %+v", example.counter, values)
			}
			if !example.allowed {
				var drops uint64
				for _, value := range values {
					if value.Verdict == "drop" && strings.HasPrefix(value.Label, example.counter) {
						drops += value.Packets
					}
				}
				if drops == 0 {
					test.Fatalf("denial lacks matching drop verdict: %+v", values)
				}
			}
			if example.untouched != "" && packets(values, example.untouched) != 0 {
				test.Fatalf("later/unmatched rule %s saw packets", example.untouched)
			}
			if example.allowed && (packets(values, "both endpoints authorized") == 0 || packets(values, "authorized replies") == 0) {
				test.Fatalf("echo lacked admission/reply counter evidence: %+v", values)
			}
			test.Logf("%s %s:%d -> %s:8080 allowed=%t result=%s counter=%q packets=%d", protocol, source, port, destination, example.allowed, result.Stage, example.counter, hits)
		})
	}
}

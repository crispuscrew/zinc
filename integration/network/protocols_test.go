package network_test

import (
	"fmt"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"golang.org/x/sys/unix"
)

func TestLiveICMP(test *testing.T) {
	for _, routed := range []bool{false, true} {
		for _, ipv6 := range []bool{false, true} {
			test.Run(fmt.Sprintf("forward=%t/ipv6=%t", routed, ipv6), func(test *testing.T) {
				lab := newLab(test, routed)
				source, destination, protocol, family := lab.client4, lab.server4, "icmp", "-4"
				if ipv6 {
					source, destination, protocol, family = lab.client6, lab.server6, "icmpv6", "-6"
				}
				argv := []string{"ping", family, "-n", "-c", "1", "-W", "1", "-I", source, destination}
				requireCommand(test, lab.client, "", argv...)
				for _, kind := range []string{"allow", "deny", "default"} {
					test.Run(kind, func(test *testing.T) {
						rule := matchingRule(protocol, source, destination, 0, false, false)
						rule.AllowAllExcept = kind == "deny"
						rules := []schema.NetworkRule{rule}
						label := "client rule[0]"
						if kind == "default" {
							rules = nil
							label = "client default"
						}
						lab.apply(test, rules, []schema.NetworkRule{matchingRule(protocol, source, destination, 0, false, true)})
						output, err := command(lab.client, "", argv...)
						if (err == nil) != (kind == "allow") {
							test.Fatalf("ping %s: %s %v", kind, output, err)
						}
						values := counters(test, lab)
						if packets(values, label) == 0 {
							test.Fatalf("no ICMP counter evidence: %+v", values)
						}
						if kind == "allow" && packets(values, "authorized replies") == 0 {
							test.Fatal("echo reply bypassed stateful rule")
						}
						test.Logf("%s %s packets=%d", protocol, kind, packets(values, label))
					})
				}
			})
		}
	}
}

func TestLiveSCTP(test *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		for _, routed := range []bool{false, true} {
			test.Run(fmt.Sprintf("forward=%t/ipv6=%t", routed, ipv6), func(test *testing.T) {
				family := unix.AF_INET
				if ipv6 {
					family = unix.AF_INET6
				}
				socket, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.IPPROTO_SCTP)
				if err == unix.EPROTONOSUPPORT || err == unix.EAFNOSUPPORT {
					test.Skipf("SCTP unavailable from kernel: socket family=%d protocol=132: %v", family, err)
				}
				if err != nil {
					test.Fatal(err)
				}
				if err := unix.Close(socket); err != nil {
					test.Fatal(err)
				}
				lab := newLab(test, routed)
				source, destination := lab.client4, lab.server4
				if ipv6 {
					source, destination = lab.client6, lab.server6
				}
				startServer(test, lab.server, "sctp", destination, 8080)
				requireControl(test, lab.client, "sctp", source, destination)
				policyCases(test, lab, "sctp", source, destination, lab.client, false)
			})
		}
	}
}

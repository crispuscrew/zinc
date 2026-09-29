package network_test

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestLiveGuestAntispoof(test *testing.T) {
	for _, kind := range []string{"source-ip", "source-mac"} {
		test.Run(kind, func(test *testing.T) {
			lab := newLab(test, true)
			startServer(test, lab.server, "tcp", lab.server4, 8080)
			requireControl(test, lab.client, "tcp", lab.client4, lab.server4)
			allow := schema.NetworkRule{From: schema.NetworkPeer{Type: schema.NetworkPeerAny},
				To: schema.NetworkPeer{Type: schema.NetworkPeerAny}, Protocols: []schema.NetworkProtocol{schema.NetworkTCP}}
			lab.apply(test, []schema.NetworkRule{allow}, []schema.NetworkRule{allow})
			verifyProbe(test, runProbe(test, lab.client, "tcp", lab.client4, lab.server4, nextPort(), 8080), true)
			source, label, family, table := lab.client4, "endpoint spoof", "inet", "zinc"
			if kind == "source-ip" {
				source = "192.0.2.11"
				requireCommand(test, lab.client, "", "ip", "addr", "add", source+"/24", "dev", "client0")
			} else {
				label, family, table = "MAC spoof", "netdev", "zinc_link"
				requireCommand(test, lab.client, "", "ip", "link", "set", "client0", "address", "02:00:00:00:00:99")
			}
			verifyProbe(test, runProbe(test, lab.client, "tcp", source, lab.server4, nextPort(), 8080), false)
			values := tableCounters(test, lab.policyNS(), family, table)
			if packets(values, label) == 0 {
				test.Fatalf("forged identity did not hit %s: %+v", label, values)
			}
			test.Logf("forged %s blocked before broad application allows; %s packets=%d", kind, label, packets(values, label))
		})
	}
}

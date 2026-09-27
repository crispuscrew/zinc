package network_test

import "testing"

func (lab laboratory) routedLinks(test *testing.T) {
	linkPair(test, lab.client, "client0", lab.router, "guest0")
	linkPair(test, lab.server, "server0", lab.router, "uplink0")
	addressLink(test, lab.client, "client0", "02:00:00:00:00:10", lab.client4, lab.client6)
	addressLink(test, lab.server, "server0", "02:00:00:00:00:20", lab.server4, lab.server6)
	addressLink(test, lab.router, "guest0", "02:00:00:00:01:01", "192.0.2.1", "2001:db8:1::1")
	addressLink(test, lab.router, "uplink0", "02:00:00:00:02:01", "198.51.100.1", "2001:db8:2::1")
	for _, setting := range []string{"net.ipv4.ip_forward", "net.ipv6.conf.all.forwarding"} {
		if string(requireCommand(test, lab.router, "", "sysctl", "-n", setting)) != "1\n" {
			test.Fatalf("test container must enable namespaced %s", setting)
		}
	}
	for _, entry := range []struct{ namespace, device, address, mac string }{
		{lab.client, "client0", "192.0.2.1", "02:00:00:00:01:01"},
		{lab.client, "client0", "2001:db8:1::1", "02:00:00:00:01:01"},
		{lab.server, "server0", "198.51.100.1", "02:00:00:00:02:01"},
		{lab.server, "server0", "2001:db8:2::1", "02:00:00:00:02:01"},
		{lab.router, "guest0", lab.client4, "02:00:00:00:00:10"},
		{lab.router, "guest0", lab.client6, "02:00:00:00:00:10"},
		{lab.router, "uplink0", lab.server4, "02:00:00:00:00:20"},
		{lab.router, "uplink0", lab.server6, "02:00:00:00:00:20"},
	} {
		neighbor(test, entry.namespace, entry.device, entry.address, entry.mac)
	}
	requireCommand(test, lab.client, "", "ip", "route", "add", "198.51.100.0/24", "via", "192.0.2.1")
	requireCommand(test, lab.server, "", "ip", "route", "add", "192.0.2.0/24", "via", "198.51.100.1")
	requireCommand(test, lab.client, "", "ip", "-6", "route", "add", "2001:db8:2::/64", "via", "2001:db8:1::1")
	requireCommand(test, lab.server, "", "ip", "-6", "route", "add", "2001:db8:1::/64", "via", "2001:db8:2::1")
}

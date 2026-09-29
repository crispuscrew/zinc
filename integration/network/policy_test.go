package network_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/nftrules"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func (lab laboratory) fixture(test *testing.T, rules, peerRules []schema.NetworkRule) (schema.AppConfig, provision.Manifest) {
	test.Helper()
	device, peerDevice, mode, kind := "client0", "client0", network.Container, schema.ZincContainer
	if lab.routed {
		device, peerDevice, mode, kind = "guest0", "uplink0", network.VirtualMachine, schema.ZincVirtualization
	}
	cfg := schema.AppConfig{Type: kind, AppNameID: "client", NetworkMeta: schema.NetworkMeta{
		Interfaces: []schema.NetworkInterface{{ID: "main", MacAddress: "02:00:00:00:00:10"}}, RulesByPriority: rules,
	}}
	manifest := provision.Manifest{Version: 1, AppNameID: "client", Generation: fmt.Sprintf("case-%d", sequence.Add(1)),
		NetworkNamespace: "/run/netns/" + lab.policyNS(), UserNamespace: "/proc/self/ns/user",
		PacketPreserving: true, Exclusive: true, StaticNeighbors: true, CompleteInventory: true, Policy: cfg.NetworkMeta,
		Topology: network.Topology{Mode: mode,
			Interfaces: []network.Attachment{{InterfaceID: "main", Device: device, MAC: "02:00:00:00:00:10", Addresses: []string{lab.client4, lab.client6}}},
			Peers: []network.Peer{{AppNameID: "server", Interfaces: []network.Attachment{{InterfaceID: "service", Device: peerDevice,
				MAC: "02:00:00:00:00:20", Addresses: []string{lab.server4, lab.server6}}},
				Policy: schema.NetworkMeta{Interfaces: []schema.NetworkInterface{{ID: "service"}}, RulesByPriority: peerRules}}},
		},
	}
	for path, target := range map[string]*uint64{manifest.NetworkNamespace: &manifest.NetworkInode, manifest.UserNamespace: &manifest.UserInode} {
		info, err := os.Stat(path)
		if err != nil {
			test.Fatal(err)
		}
		*target = info.Sys().(*syscall.Stat_t).Ino
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		test.Fatal(err)
	}
	path := filepath.Join(test.TempDir(), "client.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		test.Fatal(err)
	}
	decoded, err := provision.Decode(encoded)
	if err != nil {
		test.Fatal(err)
	}
	return cfg, decoded
}

func (lab laboratory) apply(test *testing.T, rules, peerRules []schema.NetworkRule) {
	test.Helper()
	cfg, manifest := lab.fixture(test, rules, peerRules)
	plan, err := provision.Resolve(cfg, manifest, nil)
	if err != nil {
		test.Fatal(err)
	}
	body, err := nftrules.RenderResolved(plan.Resolved)
	if err != nil {
		test.Fatal(err)
	}
	requireCommand(test, lab.policyNS(), body, "sh", "-c", provision.ApplyScript(manifest))
}

func counters(test *testing.T, lab laboratory) []nftrules.RuleCounter {
	test.Helper()
	return tableCounters(test, lab.policyNS(), "inet", "zinc")
}

func tableCounters(test *testing.T, namespace, family, table string) []nftrules.RuleCounter {
	test.Helper()
	body := requireCommand(test, namespace, "", "nft", "-j", "list", "table", family, table)
	result, err := nftrules.ParseCounters(body)
	if err != nil {
		test.Fatal(err)
	}
	return result
}

func packets(values []nftrules.RuleCounter, prefix string) uint64 {
	var total uint64
	for _, value := range values {
		if strings.HasPrefix(value.Label, prefix) {
			total += value.Packets
		}
	}
	return total
}

func matchingRule(protocol, source, destination string, sourcePort int, inbound, remote bool) schema.NetworkRule {
	from, to := schema.NetworkPeer{Type: schema.NetworkPeerSelf}, schema.NetworkPeer{Type: schema.NetworkPeerApp, AppNameID: "server"}
	if inbound {
		from, to = to, from
	}
	if remote {
		from, to = schema.NetworkPeer{Type: schema.NetworkPeerApp, AppNameID: "client"}, schema.NetworkPeer{Type: schema.NetworkPeerSelf}
		if inbound {
			from, to = to, from
		}
	}
	if strings.Contains(source, ":") {
		from.Filter.IPv6CIDR, to.Filter.IPv6CIDR = []string{source + "/128"}, []string{destination + "/128"}
	} else {
		from.Filter.IPv4CIDR, to.Filter.IPv4CIDR = []string{source + "/32"}, []string{destination + "/32"}
	}
	if sourcePort != 0 {
		from.Filter.Ports, to.Filter.Ports = []int{sourcePort}, []int{8080}
	}
	transport := schema.NetworkProtocol(strings.ToUpper(protocol))
	if protocol == "icmpv6" {
		transport = schema.NetworkICMPv6
	}
	return schema.NetworkRule{From: from, To: to, Protocols: []schema.NetworkProtocol{transport}}
}

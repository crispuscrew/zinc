package app

import (
	"net/netip"
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

func TestDNSResultsFreezeBeforeHelperPreparation(check *testing.T) {
	svc, cfg, manifest := networkFixture(check)
	cfg.NetworkMeta.RulesByPriority = []schema.NetworkRule{{From: schema.NetworkPeer{Type: schema.NetworkPeerSelf}, To: schema.NetworkPeer{Type: schema.NetworkPeerInternet}, Domains: []string{"example.com"}, Protocols: []schema.NetworkProtocol{schema.NetworkTCP}}}
	cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSUDP, Endpoint: "9.9.9.9:53"}}
	manifest.Policy = cfg.NetworkMeta
	manifest.DNSProxyAddresses = []string{"10.20.0.53"}
	manifest.DNSConfigDigest = provision.DNSDigest(cfg.NetworkMeta.DNS)
	readyProxy(check, &manifest)
	manifest.Topology.ExternalInterfaces = []string{"uplink0"}
	manifest.Topology.Hosts = []network.Host{{Interface: "host0", Device: "uplink0", Addresses: []string{"10.20.0.1"}}}
	svc.LoadNetwork = func(schema.AppConfig) (provision.Manifest, error) { return manifest, nil }
	calls := 0
	svc.Lookup = func(schema.DNSMeta, string) ([]netip.Addr, error) {
		calls++
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	plan, err := svc.launchPlan(cfg)
	if err != nil {
		check.Fatal(err)
	}
	if err := svc.freezeNetwork(&plan); err != nil {
		check.Fatal(err)
	}
	if _, _, err := svc.networkCommand(plan, qemu.Args(plan.RuntimeConfig, plan.Layout)); err != nil {
		check.Fatal(err)
	}
	if calls != 1 {
		check.Fatalf("resolved DNS %d times for one launch", calls)
	}
	if _, err := plan.Lookup(cfg.NetworkMeta.DNS, "new.example"); err == nil {
		check.Fatal("unapproved lookup added after snapshot froze")
	}
}

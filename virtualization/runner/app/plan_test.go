package app

import (
	"fmt"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func networkFixture(check *testing.T) (Service, schema.AppConfig, provision.Manifest) {
	svc, cfg := serviceFixture(check)
	svc.Paths.RunDir = "/run/zinc-test"
	cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary"}}
	manifest := provision.Manifest{Version: 1, AppNameID: cfg.AppNameID, Generation: "test-launch",
		NetworkNamespace: "/run/test.net", UserNamespace: "/run/test.user", NetworkInode: 123, UserInode: 456,
		PacketPreserving: true, Exclusive: true, StaticNeighbors: true, CompleteInventory: true, Policy: cfg.NetworkMeta,
		Topology: network.Topology{Mode: network.VirtualMachine, Interfaces: []network.Attachment{
			{InterfaceID: "primary", Device: "tap0", MAC: "02:00:00:00:00:01", Addresses: []string{"10.20.0.2"}},
		}},
	}
	return svc, cfg, manifest
}

func TestProvisionedPlanBindsMACWithoutChangingAuthoredConfig(check *testing.T) {
	svc, cfg, manifest := networkFixture(check)
	svc.LoadNetwork = func(received schema.AppConfig) (provision.Manifest, error) {
		if received.NetworkMeta.Interfaces[0].MacAddress != "" {
			check.Fatal("authored MAC changed before manifest load")
		}
		return manifest, nil
	}
	args, rules, err := svc.Plan(cfg)
	if err != nil {
		check.Fatal(err)
	}
	text := strings.Join(args, " ")
	if !strings.Contains(text, "tap,id=net0,ifname=tap0") || !strings.Contains(text, "mac=02:00:00:00:00:01") || rules == "" {
		check.Fatal(text, rules)
	}
	if cfg.NetworkMeta.Interfaces[0].MacAddress != "" {
		check.Fatal("authored config mutated")
	}
	if _, err := os.Stat(svc.Paths.StateDir); !os.IsNotExist(err) {
		check.Fatal("plan created state")
	}
}

func TestAllRuntimePublicationsMustMatchManifest(check *testing.T) {
	svc, cfg, manifest := networkFixture(check)
	svc.Options.ForwardPorts = []vmoptions.PortForward{{HostPort: 2222, GuestPort: 22}}
	svc.LoadNetwork = func(schema.AppConfig) (provision.Manifest, error) { return manifest, nil }
	if _, _, err := svc.Plan(cfg); err == nil || !strings.Contains(err.Error(), "mappings") {
		check.Fatalf("got %v", err)
	}
	manifest.Publications = []provision.Publication{{Protocol: "TCP", BindAddress: "0.0.0.0", HostPort: 2222, GuestPort: 22, InterfaceID: "primary"}}
	if _, _, err := svc.Plan(cfg); err == nil {
		check.Fatal("wildcard satisfied loopback request")
	}
	manifest.Publications[0].BindAddress = "127.0.0.1"
	if _, _, err := svc.Plan(cfg); err != nil {
		check.Fatal(err)
	}
}

func TestManifestLoadPrecedesQEMUTransportValidation(check *testing.T) {
	svc, cfg, _ := networkFixture(check)
	cfg.NetworkMeta.RulesByPriority = []schema.NetworkRule{{From: schema.NetworkPeer{Type: schema.NetworkPeerSelf}, To: schema.NetworkPeer{Type: schema.NetworkPeerInternet}, Protocols: []schema.NetworkProtocol{schema.NetworkSCTP}}}
	svc.LoadNetwork = func(schema.AppConfig) (provision.Manifest, error) {
		return provision.Manifest{}, fmt.Errorf("missing authoritative manifest")
	}
	if _, _, err := svc.Plan(cfg); err == nil || !strings.Contains(err.Error(), "authoritative") {
		check.Fatalf("got %v", err)
	}
}

func TestDNSLookupIsInjectedAndReceivesOwnerPolicy(check *testing.T) {
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
	called := false
	svc.Lookup = func(meta schema.DNSMeta, name string) ([]netip.Addr, error) {
		called = true
		if !reflect.DeepEqual(meta, cfg.NetworkMeta.DNS) || name != "example.com" {
			check.Fatal(meta, name)
		}
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	_, _, err := svc.Plan(cfg)
	if err != nil {
		check.Fatal(err)
	}
	if !called {
		check.Fatal("lookup not called")
	}
}

func TestPlanOmitsRawSecretsAndNeverConnectsAudio(check *testing.T) {
	svc, cfg := serviceFixture(check)
	svc.Paths.RunDir = "/run/zinc-test"
	cfg.AudioMeta.Monitor.PipeWireDefault = true
	cfg.RunnerFlags = []string{"-object", "secret,id=key,data=do-not-print"}
	args, _, err := svc.Plan(cfg)
	if err != nil {
		check.Fatal(err)
	}
	text := strings.Join(args, " ")
	if strings.Contains(text, "do-not-print") || !strings.Contains(text, "za.plan.0") {
		check.Fatal(text)
	}
}

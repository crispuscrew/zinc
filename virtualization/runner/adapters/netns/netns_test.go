package netns

import (
	"reflect"
	"strings"
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func vmFixture() (schema.AppConfig, provision.Manifest, []string) {
	cfg := schema.AppConfig{Type: schema.ZincVirtualization, AppNameID: "guest", NetworkMeta: schema.NetworkMeta{
		Interfaces:      []schema.NetworkInterface{{ID: "main", MacAddress: "02:00:00:00:00:01"}},
		RulesByPriority: []schema.NetworkRule{{From: schema.NetworkPeer{Type: schema.NetworkPeerSelf}, To: schema.NetworkPeer{Type: schema.NetworkPeerAny}}},
	}}
	manifest := provision.Manifest{Version: 1, AppNameID: "guest", Generation: "launch-1", NetworkNamespace: "/run/zinc/guest.net", UserNamespace: "/run/zinc/guest.user",
		NetworkInode: 123, UserInode: 456, PacketPreserving: true, Exclusive: true, StaticNeighbors: true, CompleteInventory: true, Policy: cfg.NetworkMeta,
		Topology: network.Topology{Mode: network.VirtualMachine, Interfaces: []network.Attachment{{InterfaceID: "main", Device: "ztap0", MAC: "02:00:00:00:00:01", Addresses: []string{"10.0.0.2"}}}},
	}
	argv := []string{"qemu-system-x86_64", "-netdev", "tap,id=net0,ifname=ztap0,script=no,downscript=no", "-device", "virtio-net-pci,netdev=net0,mac=02:00:00:00:00:01"}
	return cfg, manifest, argv
}

func TestEmptyInterfacesRequireNoNIC(t *testing.T) {
	argv := []string{"qemu-system-x86_64", "-nodefaults"}
	command, ruleset, err := Command(schema.AppConfig{}, argv, "")
	if err != nil || ruleset != "" || !reflect.DeepEqual(command, argv) {
		t.Fatal("isolated command changed")
	}
	for _, flag := range []string{"-netdev", "-nic", "-net", "--netdev", "--nic=user", "-netdev=user,id=net0"} {
		if _, _, err := Command(schema.AppConfig{}, append(argv, flag, "user"), ""); err == nil {
			t.Fatal("empty interfaces accepted NIC")
		}
	}
	if _, _, err := Command(schema.AppConfig{}, append(argv, "-device", "virtio-net-pci"), ""); err == nil {
		t.Fatal("empty interfaces accepted an implicit NIC")
	}
}

func TestCommandLoadsGuestPacketPolicyBeforeStartup(t *testing.T) {
	cfg, manifest, argv := vmFixture()
	command, ruleset, err := CommandResolved(cfg, argv, "", manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if command[0] != Binary {
		t.Fatal("wrong namespace entry program")
	}
	script := command[len(command)-1]
	if strings.Index(script, "nft -f -") > strings.Index(script, "'qemu-system-x86_64'") || !strings.HasPrefix(script, "set -eu") || !strings.Contains(script, "trap ") {
		t.Fatal("missing startup/rollback ordering")
	}
	if !strings.Contains(ruleset, "jump owner_forward") || !strings.Contains(ruleset, "table netdev zinc_link") {
		t.Fatal("filters sockets instead of guest packets")
	}
	if strings.Contains(script, "mount --bind") {
		t.Fatal("TAP guest must not modify host resolv.conf")
	}
}

func TestCommandQuotesEveryArgument(t *testing.T) {
	cfg, manifest, argv := vmFixture()
	argv = append(argv, "-drive", "file=/home/a b/owner's disk.qcow2")
	command, _, err := CommandResolved(cfg, argv, "", manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command[len(command)-1], `'file=/home/a b/owner'\''s disk.qcow2'`) {
		t.Fatal("shell argv quote broken")
	}
}

func TestBackendIdentityCannotDrift(t *testing.T) {
	cfg, manifest, original := vmFixture()
	for _, backend := range []string{"user,id=net0,hostfwd=tcp:127.0.0.1:2222-:22", "tap,id=net0,ifname=wrong,script=no,downscript=no", "tap,id=net0,ifname=ztap0,script=/tmp/unsafe,downscript=no"} {
		argv := append([]string{}, original...)
		argv[2] = backend
		if _, _, err := CommandResolved(cfg, argv, "", manifest, nil); err == nil {
			t.Fatalf("accepted %s", backend)
		}
	}
	argv := append([]string{}, original...)
	argv[4] = "virtio-net-pci,netdev=net0,mac=02:00:00:00:00:ff"
	if _, _, err := CommandResolved(cfg, argv, "", manifest, nil); err == nil {
		t.Fatal("ignored MAC drift")
	}
	if _, _, err := CommandResolved(cfg, original[:1], "", manifest, nil); err == nil {
		t.Fatal("ignored absent attachments")
	}
}

func TestNoAmbientDNSOrImplicitResolverMount(t *testing.T) {
	cfg, manifest, argv := vmFixture()
	cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSTLS, Endpoint: "1.1.1.1"}}
	manifest.Policy = cfg.NetworkMeta
	if _, _, err := CommandResolved(cfg, argv, "resolver-file", manifest, nil); err == nil {
		t.Fatal("TLS endpoint became plaintext resolver")
	}
	manifest.DNSProxyAddresses, manifest.DNSConfigDigest = []string{"10.0.0.1"}, provision.DNSDigest(cfg.NetworkMeta.DNS)
	if _, _, err := CommandResolved(cfg, argv, "resolver-file", manifest, nil); err == nil || !strings.Contains(err.Error(), "dns_control_socket") {
		t.Fatalf("unverified proxy must not allow startup: %v", err)
	}
	if ResolvConf(cfg) != "" {
		t.Fatal("TAP configuration wrote host resolver")
	}
}

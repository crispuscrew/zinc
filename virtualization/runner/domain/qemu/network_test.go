package qemu

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func TestDeclaredNICsAndForwards(t *testing.T) {
	cfg, layout := fixture()
	cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary"}, {ID: "private", MacAddress: "02:11:22:33:44:55"}}
	layout.Runtime.ForwardPorts = []vmoptions.PortForward{{HostPort: 2222, GuestPort: 22}, {Protocol: "UDP", HostPort: 5353, GuestPort: 53, Interface: "private"}}
	if err := Validate(cfg, layout); err != nil {
		t.Fatal(err)
	}
	args := Args(cfg, layout)
	text := strings.Join(args, " ")
	if strings.Count(text, "-netdev") != 2 {
		t.Fatal(text)
	}
	for _, want := range []string{"restrict=on", "hostfwd=tcp:127.0.0.1:2222-:22", "hostfwd=udp:127.0.0.1:5353-:53", "mac=02:11:22:33:44:55"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s", want)
		}
	}
	layout.Namespaced = true
	if !strings.Contains(strings.Join(Args(cfg, layout), " "), "hostfwd=tcp:127.0.0.1:2222-:22") {
		t.Fatal("requested host bind lost before network adapter could publish it")
	}
}

func TestTAPRequiredForPacketProtocolsAndScopedPolicy(t *testing.T) {
	cfg, layout := fixture()
	cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary"}}
	cfg.NetworkMeta.RulesByPriority = []schema.NetworkRule{{Protocols: []schema.NetworkProtocol{schema.NetworkSCTP}}}
	if err := Validate(cfg, layout); err == nil || !strings.Contains(err.Error(), "TAP") {
		t.Fatalf("got %v", err)
	}
	layout.NetworkAttachments = []NetworkAttachment{{InterfaceID: "primary", TapName: "vm-tap0"}}
	if err := Validate(cfg, layout); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(Args(cfg, layout), " ")
	if !strings.Contains(text, "tap,id=net0,ifname=vm-tap0,script=no,downscript=no") || strings.Contains(text, "user,id=") {
		t.Fatal(text)
	}
	layout.NetworkAttachments[0].InterfaceID = "missing"
	if Validate(cfg, layout) == nil {
		t.Fatal("unknown NIC accepted")
	}
}

func TestIncompleteTAPAndMissingForwardNICRejected(t *testing.T) {
	cfg, layout := fixture()
	cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary"}, {ID: "second"}}
	layout.NetworkAttachments = []NetworkAttachment{{InterfaceID: "primary", TapName: "tap0"}}
	if Validate(cfg, layout) == nil {
		t.Fatal("mixed TAP/slirp accepted")
	}
	layout.NetworkAttachments = nil
	layout.Runtime.ForwardPorts = []vmoptions.PortForward{{HostPort: 2222, GuestPort: 22, Interface: "missing"}}
	if Validate(cfg, layout) == nil {
		t.Fatal("forward into absent NIC accepted")
	}
}

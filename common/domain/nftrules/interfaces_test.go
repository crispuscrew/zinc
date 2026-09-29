package nftrules

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestMultipleInterfacesAreExactNotFirstInterfaceWins(t *testing.T) {
	resolved := fixture()
	resolved.Config.NetworkMeta.Interfaces = append(resolved.Config.NetworkMeta.Interfaces, schema.NetworkInterface{ID: "service"})
	resolved.Topology.Interfaces = append(resolved.Topology.Interfaces, network.Attachment{
		InterfaceID: "service", Device: "zsvc0", MAC: "02:00:00:00:00:03", Addresses: []string{"10.20.0.2", "fd00:20::2"},
	})
	resolved.Config.NetworkMeta.RulesByPriority[0].From.Interface = "service"
	result := render(t, resolved)
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, "client rule[0]") && !strings.Contains(line, `name "zsvc0"`) {
			t.Fatalf("logical interface disappeared: %s", line)
		}
	}
	if !strings.Contains(result, "10.20.0.2/32") || !strings.Contains(result, "fd00:20::2/128") {
		t.Fatal("second interface addresses absent")
	}
}

func TestGenerationChangesStatefulAdmissionMark(t *testing.T) {
	resolved := fixture()
	resolved.Generation = "launch-1"
	before := render(t, resolved)
	resolved.Generation = "launch-2"
	after := render(t, resolved)
	mark := func(body string) string {
		for _, line := range strings.Split(body, "\n") {
			if strings.Contains(line, "ct state established,related") {
				return line
			}
		}
		return ""
	}
	if mark(before) == "" || mark(before) == mark(after) {
		t.Fatal("new provisioning generation trusted old conntrack admission")
	}
}

func TestUnknownAndIncompatibleTransportNeverSilentlyNarrows(t *testing.T) {
	for _, protocols := range [][]schema.NetworkProtocol{nil, {schema.NetworkICMP}, {schema.NetworkICMPv6}, {schema.NetworkGRE}, {schema.NetworkESP}, {schema.NetworkAH}} {
		resolved := fixture()
		resolved.Config.NetworkMeta.RulesByPriority[0].Protocols = protocols
		resolved.Config.NetworkMeta.RulesByPriority[0].To.Filter.Ports = []int{443}
		if _, err := RenderResolved(resolved); err == nil {
			t.Fatalf("accepted ports on %v", protocols)
		}
	}
}

func TestSelfSpoofCannotUseEstablishedShortcut(t *testing.T) {
	resolved := fixture()
	resolved.Topology.Mode = network.VirtualMachine
	result := render(t, resolved)
	start := strings.Index(result, " chain forward {")
	body := result[start:]
	guard := strings.Index(body, "self route spoof")
	state := strings.Index(body, "authorized replies")
	if guard < 0 || state < guard {
		t.Fatal("conntrack bypasses endpoint identity validation")
	}
}

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestImportPortsKeepPeersAndProtocols(t *testing.T) {
	app := importOne(t, "services:\n  app:\n    ports: ['8080:80', '9000:9000/udp', '127.0.0.1:8081:8081/tcp']\n    expose: [5432]\n")
	network := app.Config.NetworkMeta
	if len(network.Interfaces) != 1 || len(network.RulesByPriority) != 3 {
		t.Fatal(network)
	}
	for index, expected := range []struct {
		peer     schema.NetworkPeerType
		port     int
		protocol schema.NetworkProtocol
	}{
		{schema.NetworkPeerAny, 9000, schema.NetworkUDP}, {schema.NetworkPeerHost, 8081, schema.NetworkTCP}, {schema.NetworkPeerAnyApp, 5432, schema.NetworkTCP},
	} {
		rule := network.RulesByPriority[index]
		if rule.From.Type != expected.peer || rule.To.Type != schema.NetworkPeerSelf || !slices.Equal(rule.To.Filter.Ports, []int{expected.port}) || !slices.Equal(rule.Protocols, []schema.NetworkProtocol{expected.protocol}) {
			t.Fatalf("rule %d: %+v", index, rule)
		}
	}
	if !hasNote(app, "translation is not representable") || !hasNote(app, "bound to loopback") {
		t.Fatal(app.Notes)
	}
}

func TestImportLongSyntax(t *testing.T) {
	app := importOne(t, `services:
  web:
    ports:
      - target: 5000
        published: "5000"
        protocol: udp
        host_ip: 127.0.0.1
    volumes:
      - type: bind
        source: /srv/site
        target: /site
        read_only: true
`)
	if len(app.Config.NetworkMeta.RulesByPriority) != 1 {
		t.Fatal(app)
	}
	rule := app.Config.NetworkMeta.RulesByPriority[0]
	if rule.From.Type != schema.NetworkPeerHost || rule.Protocols[0] != schema.NetworkUDP {
		t.Fatal(rule)
	}
	if len(app.Config.Volumes) != 1 || app.Config.Volumes[0].Writable {
		t.Fatal(app.Config.Volumes)
	}
}

func TestImportUnrepresentableBindingsGrantNothing(t *testing.T) {
	for _, value := range []string{"192.0.2.1:8080:8080", "8080:80", "80-90:80-90", "9000:9000/unknown"} {
		app := importOne(t, "services:\n  app:\n    ports: ['"+value+"']\n")
		if len(app.Config.NetworkMeta.Interfaces) != 0 || !hasNote(app, "dropped") {
			t.Fatal(app)
		}
	}
}

func TestExportNetworkScopesAndLosses(t *testing.T) {
	cfg := containerApp("app")
	cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary"}}
	for _, peer := range []schema.NetworkPeerType{schema.NetworkPeerHost, schema.NetworkPeerAnyApp, schema.NetworkPeerAny} {
		cfg.NetworkMeta.RulesByPriority = append(cfg.NetworkMeta.RulesByPriority, schema.NetworkRule{From: schema.NetworkPeer{Type: peer}, To: schema.NetworkPeer{Type: schema.NetworkPeerSelf, Filter: schema.NetworkPeerFilter{Ports: []int{8080}}}, Protocols: []schema.NetworkProtocol{schema.NetworkUDP}})
	}
	service, notes := exportApp(t, cfg)
	if !slices.Equal(service.Ports, StringList{"127.0.0.1:8080:8080/udp", "8080:8080/udp"}) || !slices.Equal(service.Expose, StringList{"8080/udp"}) {
		t.Fatal(service)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "EGRESS LOCK-DOWN IS NOT REPRESENTED") {
		t.Fatal(notes)
	}
	cfg.NetworkMeta.RulesByPriority[0].AllowAllExcept = true
	service, notes = exportApp(t, cfg)
	if len(service.Ports)+len(service.Expose) != 0 || !strings.Contains(strings.Join(notes, "\n"), "exclusions") {
		t.Fatal("ordered deny was lost into a grant")
	}
}

func TestExportDoesNotWidenRestrictedSources(t *testing.T) {
	cfg := containerApp("app")
	cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary"}}
	cfg.NetworkMeta.RulesByPriority = []schema.NetworkRule{{From: schema.NetworkPeer{Type: schema.NetworkPeerInternet, Filter: schema.NetworkPeerFilter{IPv4CIDR: []string{"1.1.1.1/32"}}}, To: schema.NetworkPeer{Type: schema.NetworkPeerSelf, Filter: schema.NetworkPeerFilter{Ports: []int{443}}}, Protocols: []schema.NetworkProtocol{schema.NetworkTCP}}}
	service, notes := exportApp(t, cfg)
	if len(service.Ports) != 0 || !strings.Contains(strings.Join(notes, "\n"), "restriction") {
		t.Fatal(service, notes)
	}
}

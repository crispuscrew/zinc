package compose

import (
	"net"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func importNetwork(service Service, note func(string, ...any)) schema.NetworkMeta {
	var network schema.NetworkMeta
	if service.NetworkMode == "none" {
		return network
	}
	if service.NetworkMode != "" {
		note("network_mode %q was not imported; no network grants inferred", service.NetworkMode)
	}
	for _, source := range []struct {
		specs StringList
		peer  schema.NetworkPeerType
	}{
		{service.Ports, schema.NetworkPeerAny}, {service.Expose, schema.NetworkPeerAnyApp},
	} {
		for _, spec := range source.specs {
			peer, port, protocol, ok := importedPort(spec, source.peer, note)
			if !ok {
				continue
			}
			network.RulesByPriority = append(network.RulesByPriority, schema.NetworkRule{
				From:      schema.NetworkPeer{Type: peer},
				To:        schema.NetworkPeer{Type: schema.NetworkPeerSelf, Interface: "primary", Filter: schema.NetworkPeerFilter{Ports: []int{port}}},
				Protocols: []schema.NetworkProtocol{protocol},
			})
		}
	}
	if len(network.RulesByPriority) > 0 {
		network.Interfaces = []schema.NetworkInterface{{ID: "primary"}}
	}
	for _, server := range service.DNS {
		if len(network.Interfaces) == 0 || net.ParseIP(server) == nil {
			note("dns %q was not imported: it needs an explicit NIC and an IP resolver", server)
			continue
		}
		network.DNS.ResolversByPriority = append(network.DNS.ResolversByPriority, schema.DNSResolver{Protocol: schema.DNSUDP, Endpoint: net.JoinHostPort(server, "53")})
	}
	return network
}

func importedPort(spec string, peer schema.NetworkPeerType, note func(string, ...any)) (schema.NetworkPeerType, int, schema.NetworkProtocol, bool) {
	address, protocolText, explicit := strings.Cut(spec, "/")
	if !explicit {
		protocolText = "tcp"
	}
	protocol := schema.NetworkProtocol(strings.ToUpper(protocolText))
	if protocol != schema.NetworkTCP && protocol != schema.NetworkUDP && protocol != schema.NetworkSCTP {
		note("port %q was dropped: unsupported transport %q", spec, protocolText)
		return peer, 0, protocol, false
	}
	parts := strings.Split(address, ":")
	guest, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || guest < 1 || guest > 65535 {
		note("port %q was dropped: want one port in 1-65535", spec)
		return peer, 0, protocol, false
	}
	if len(parts) > 1 {
		host, err := strconv.Atoi(parts[len(parts)-2])
		if err != nil || host != guest {
			note("port %q was dropped: host-to-guest port translation is not representable in the app schema", spec)
			return peer, 0, protocol, false
		}
	}
	if len(parts) > 2 {
		bind := strings.Trim(strings.Join(parts[:len(parts)-2], ":"), "[]")
		address := net.ParseIP(bind)
		if address == nil {
			note("port %q was dropped: invalid bind address", spec)
			return peer, 0, protocol, false
		}
		switch {
		case address.IsLoopback():
			peer = schema.NetworkPeerHost
			note("port %q was bound to loopback and remains Host -> Self only", spec)
		case !address.IsUnspecified():
			note("port %q was dropped: binding one host IP cannot be represented without a host interface decision", spec)
			return peer, 0, protocol, false
		}
	}
	return peer, guest, protocol, true
}

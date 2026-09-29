package validate

import (
	"fmt"
	"net"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func checkInterfaces(interfaces []schema.NetworkInterface, add addFunc) map[string]bool {
	identifiers := make(map[string]bool)
	addresses := make(map[string]bool)
	for index, iface := range interfaces {
		field := fmt.Sprintf("NetworkMeta.Interfaces[%d]", index)
		if !nameRE.MatchString(iface.ID) {
			add("%s.ID %q: must be lowercase [a-z0-9._-] starting alphanumeric", field, iface.ID)
		}
		if identifiers[iface.ID] {
			add("%s.ID %q: duplicate interface ID", field, iface.ID)
		}
		identifiers[iface.ID] = true
		if iface.MacAddress == "" {
			continue
		}
		checkMAC(field+".MacAddress", iface.MacAddress, add)
		address := strings.ToLower(iface.MacAddress)
		if addresses[address] {
			add("%s.MacAddress %q: duplicate MAC address", field, iface.MacAddress)
		}
		addresses[address] = true
	}
	return identifiers
}

func checkMAC(field, value string, add addFunc) {
	address, err := net.ParseMAC(value)
	if err != nil || len(address) != 6 || len(value) != 17 || strings.Count(value, ":") != 5 {
		add("%s %q: must be six colon-separated two-digit hex octets", field, value)
		return
	}
	if address[0]&1 != 0 {
		add("%s %q: multicast addresses cannot identify a NIC; use unicast", field, value)
	}
	if value == "00:00:00:00:00:00" {
		add("%s: the all-zero address is not usable", field)
	}
}

// Existence is checked against the launch host, not the author's machine.
func validHostInterface(name string) bool {
	return len(name) <= 15 && name != "." && name != ".." && ifaceRE.MatchString(name)
}

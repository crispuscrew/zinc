package network

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var deviceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,14}$`)

func ValidID(value string) bool {
	return identifier.MatchString(value) && value != "." && value != ".."
}
func ValidDevice(value string) bool { return deviceName.MatchString(value) && value != "lo" }

func Validate(resolved Resolved) error {
	if err := ValidateTopology(resolved); err != nil {
		return err
	}
	if err := policy(resolved, resolved.Config.AppNameID, resolved.Config.NetworkMeta); err != nil {
		return err
	}
	for _, peer := range resolved.Topology.Peers {
		if err := policy(resolved, peer.AppNameID, peer.Policy); err != nil {
			return err
		}
	}
	return nil
}

func ValidateTopology(resolved Resolved) error {
	if !ValidID(resolved.Config.AppNameID) {
		return fmt.Errorf("invalid app identity")
	}
	topology := resolved.Topology
	if topology.Mode != Container && topology.Mode != VirtualMachine {
		return fmt.Errorf("require container or tap topology")
	}
	if err := attachments(resolved.Config.NetworkMeta, topology.Interfaces, true); err != nil {
		return err
	}
	seen := map[string]bool{resolved.Config.AppNameID: true}
	addresses := map[string]bool{}
	for _, attachment := range topology.Interfaces {
		for _, address := range attachment.Addresses {
			addresses[address] = true
		}
	}
	for _, peer := range topology.Peers {
		if !ValidID(peer.AppNameID) || seen[peer.AppNameID] {
			return fmt.Errorf("invalid or duplicate peer %q", peer.AppNameID)
		}
		seen[peer.AppNameID] = true
		if err := attachments(peer.Policy, peer.Interfaces, false); err != nil {
			return fmt.Errorf("peer %s: %w", peer.AppNameID, err)
		}
		for _, attachment := range peer.Interfaces {
			for _, address := range attachment.Addresses {
				if addresses[address] {
					return fmt.Errorf("ambiguous endpoint address %s", address)
				}
				addresses[address] = true
			}
		}
	}
	for _, host := range topology.Hosts {
		if !deviceName.MatchString(host.Interface) || !ValidDevice(host.Device) {
			return fmt.Errorf("invalid host interface mapping")
		}
		if err := validateAddresses(host.Addresses); err != nil {
			return err
		}
		for _, address := range host.Addresses {
			if addresses[address] {
				return fmt.Errorf("host address overlaps app: %s", address)
			}
		}
	}
	for _, device := range topology.ExternalInterfaces {
		if !ValidDevice(device) {
			return fmt.Errorf("invalid external device %q", device)
		}
	}
	return nil
}

func attachments(meta schema.NetworkMeta, entries []Attachment, distinctDevices bool) error {
	if len(entries) != len(meta.Interfaces) {
		return fmt.Errorf("manifest must map every declared interface exactly")
	}
	seen, devices, addresses := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		if !ValidID(entry.InterfaceID) || seen[entry.InterfaceID] || !ValidDevice(entry.Device) || distinctDevices && devices[entry.Device] {
			return fmt.Errorf("invalid or duplicate interface mapping %q", entry.InterfaceID)
		}
		seen[entry.InterfaceID], devices[entry.Device] = true, true
		mac, err := net.ParseMAC(entry.MAC)
		if err != nil || len(mac) != 6 || mac[0]&1 != 0 || entry.MAC == "00:00:00:00:00:00" || mac.String() != entry.MAC {
			return fmt.Errorf("interface %s requires canonical nonzero unicast MAC", entry.InterfaceID)
		}
		if err := validateAddresses(entry.Addresses); err != nil {
			return err
		}
		for _, address := range entry.Addresses {
			if addresses[address] {
				return fmt.Errorf("duplicate assigned address %s", address)
			}
			addresses[address] = true
		}
	}
	for _, iface := range meta.Interfaces {
		if !seen[iface.ID] {
			return fmt.Errorf("unprovisioned interface %q", iface.ID)
		}
		for _, entry := range entries {
			if entry.InterfaceID == iface.ID && iface.MacAddress != "" && !strings.EqualFold(iface.MacAddress, entry.MAC) {
				return fmt.Errorf("MAC mismatch for interface %s", iface.ID)
			}
		}
		delete(seen, iface.ID)
	}
	return nil
}

func validateAddresses(addresses []string) error {
	if len(addresses) == 0 {
		return fmt.Errorf("assigned addresses are required")
	}
	for _, value := range addresses {
		address, err := netip.ParseAddr(value)
		if err != nil || address.Zone() != "" || address.Is4In6() || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() || address.String() != value {
			return fmt.Errorf("invalid assigned unicast address %q", value)
		}
	}
	return nil
}

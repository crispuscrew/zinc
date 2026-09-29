package network

import (
	"fmt"
	"net/netip"

	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

// Publication attests an existing host-side mapping; it never creates one.
type Publication struct {
	Protocol    string `json:"protocol"`
	BindAddress string `json:"bind_address"`
	HostPort    int    `json:"host_port"`
	GuestPort   int    `json:"guest_port"`
	InterfaceID string `json:"interface_id"`
}

func validatePublications(manifest Manifest) error {
	seen := map[Publication]bool{}
	for _, entry := range manifest.Publications {
		address, err := netip.ParseAddr(entry.BindAddress)
		if err != nil || address.Zone() != "" || address.Is4In6() || address.IsMulticast() || (entry.Protocol != "TCP" && entry.Protocol != "UDP") ||
			entry.HostPort < 1 || entry.HostPort > 65535 || entry.GuestPort < 1 || entry.GuestPort > 65535 || seen[entry] {
			return fmt.Errorf("invalid or duplicate provisioned publication")
		}
		found := false
		for _, iface := range manifest.Topology.Interfaces {
			found = found || entry.InterfaceID == iface.InterfaceID
		}
		if !found {
			return fmt.Errorf("publication references absent interface %s", entry.InterfaceID)
		}
		seen[entry] = true
	}
	return nil
}

// CheckForwards requires an exact mapping including the requested host bind.
// A loopback request must never be satisfied by a wildcard provisioned bind.
func CheckForwards(manifest Manifest, forwards []vmoptions.PortForward) error {
	if err := validatePublications(manifest); err != nil {
		return err
	}
	if len(forwards) != len(manifest.Publications) {
		return fmt.Errorf("host port mappings must be explicitly provisioned and match runtime options")
	}
	remaining := map[Publication]bool{}
	for _, entry := range manifest.Publications {
		remaining[entry] = true
	}
	for _, forward := range forwards {
		iface := forward.Interface
		if iface == "" && len(manifest.Policy.Interfaces) > 0 {
			iface = manifest.Policy.Interfaces[0].ID
		}
		entry := Publication{Protocol: forward.Transport(), BindAddress: forward.Bind(), HostPort: forward.HostPort, GuestPort: forward.GuestPort, InterfaceID: iface}
		if !remaining[entry] {
			return fmt.Errorf("unprovisioned %s forward %s:%d to %s:%d", entry.Protocol, entry.BindAddress, entry.HostPort, iface, entry.GuestPort)
		}
		delete(remaining, entry)
	}
	return nil
}

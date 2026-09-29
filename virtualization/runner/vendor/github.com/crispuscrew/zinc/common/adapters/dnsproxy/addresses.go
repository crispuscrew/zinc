package dnsproxy

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
)

const maxListeners = 16

func listenerAddresses(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > maxListeners {
		return nil, fmt.Errorf("DNS proxy requires 1..%d explicit listener addresses", maxListeners)
	}
	var result []string
	for _, value := range values {
		if address, err := netip.ParseAddr(value); err == nil {
			value = net.JoinHostPort(address.String(), "53")
		}
		endpoint, err := netip.ParseAddrPort(value)
		if err != nil {
			return nil, fmt.Errorf("invalid numeric DNS listener %q", value)
		}
		address := endpoint.Addr()
		unicast := address.IsGlobalUnicast() || address.IsLoopback() || address.IsLinkLocalUnicast()
		if !unicast || address.Is4In6() || address.Zone() != "" {
			return nil, fmt.Errorf("DNS listener must be an explicit unicast address")
		}
		value = endpoint.String()
		if slices.Contains(result, value) {
			return nil, fmt.Errorf("duplicate DNS listener %q", value)
		}
		result = append(result, value)
	}
	slices.Sort(result)
	return result, nil
}

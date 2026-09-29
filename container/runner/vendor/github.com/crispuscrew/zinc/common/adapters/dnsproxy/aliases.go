package dnsproxy

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/miekg/dns"
)

func aliasRecords(answer *dns.Msg, name string, kind uint16) ([]netip.Addr, string, error) {
	var result []netip.Addr
	var target string
	seen := map[netip.Addr]bool{}
	for _, record := range answer.Answer {
		header := record.Header()
		if !strings.EqualFold(header.Name, name) || header.Class != dns.ClassINET {
			continue
		}
		var address netip.Addr
		switch value := record.(type) {
		case *dns.CNAME:
			if _, valid := dns.IsDomainName(value.Target); !valid || (target != "" && !strings.EqualFold(target, value.Target)) {
				return nil, "", fmt.Errorf("invalid or conflicting DNS CNAME")
			}
			target = value.Target
		case *dns.A:
			if kind == dns.TypeA {
				address, _ = netip.AddrFromSlice(value.A.To4())
			}
		case *dns.AAAA:
			if kind == dns.TypeAAAA {
				address, _ = netip.AddrFromSlice(value.AAAA.To16())
			}
		}
		if address.IsValid() && !seen[address] {
			seen[address] = true
			result = append(result, address.Unmap())
		}
	}
	if target != "" && len(result) != 0 {
		return nil, "", fmt.Errorf("DNS owner has both CNAME and address records")
	}
	return result, target, nil
}

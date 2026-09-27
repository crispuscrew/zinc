package dnsproxy

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

// Lookup matches adapters/network.Lookup. It returns no partial result on failure.
// NXDOMAIN and successful NODATA return an empty result without host resolution.
func Lookup(meta schema.DNSMeta, name string) ([]netip.Addr, error) {
	resolver, err := New(meta)
	if err != nil {
		return nil, err
	}
	return resolver.Lookup(context.Background(), name)
}

// Lookup resolves A and AAAA, following at most eight CNAME links per family.
func (resolver *Resolver) Lookup(ctx context.Context, name string) ([]netip.Addr, error) {
	name = dns.Fqdn(name)
	if _, valid := dns.IsDomainName(name); !valid || name == "." {
		return nil, fmt.Errorf("invalid lookup name")
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	var result []netip.Addr
	for _, kind := range []uint16{dns.TypeA, dns.TypeAAAA} {
		addresses, missing, err := resolver.family(ctx, name, kind)
		if err != nil {
			return nil, err
		}
		if missing {
			if len(result) != 0 {
				return nil, fmt.Errorf("inconsistent DNS NXDOMAIN across address families")
			}
			return nil, nil
		}
		result = append(result, addresses...)
	}
	return result, nil
}

func (resolver *Resolver) family(ctx context.Context, name string, kind uint16) ([]netip.Addr, bool, error) {
	visited := map[string]bool{strings.ToLower(name): true}
	for {
		query := new(dns.Msg)
		query.SetQuestion(name, kind)
		answer, err := resolver.Exchange(ctx, query)
		if err != nil {
			return nil, false, err
		}
		if answer.Rcode == dns.RcodeNameError {
			return nil, true, nil
		}
		requested := name
		for {
			addresses, target, err := aliasRecords(answer, name, kind)
			if err != nil {
				return nil, false, err
			}
			if len(addresses) != 0 {
				return addresses, false, nil
			}
			if target == "" {
				if strings.EqualFold(requested, name) {
					return nil, false, nil
				}
				break
			}
			key := strings.ToLower(target)
			if visited[key] || len(visited) > maxAliases {
				return nil, false, fmt.Errorf("DNS CNAME cycle or chain limit")
			}
			visited[key], name = true, target
		}
	}
}

package network

import (
	"fmt"
	"sort"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func resolveDomains(resolved *domain.Resolved, lookup Lookup) error {
	policies := []domain.Peer{{AppNameID: resolved.Config.AppNameID, Policy: resolved.Config.NetworkMeta}}
	policies = append(policies, resolved.Topology.Peers...)
	for _, owner := range policies {
		resolved.Domains[owner.AppNameID] = map[int][]string{}
		for index, rule := range owner.Policy.RulesByPriority {
			if len(rule.Domains) == 0 {
				continue
			}
			if lookup == nil || len(owner.Policy.DNS.ResolversByPriority) == 0 {
				return fmt.Errorf("%s rule[%d]: approved DNS resolver integration required for Domains; no host fallback", owner.AppNameID, index)
			}
			if _, err := domain.Upstreams(owner.Policy.DNS); err != nil {
				return err
			}
			addresses, err := resolveRule(owner.Policy.DNS, rule.Domains, lookup)
			if err != nil {
				return fmt.Errorf("%s rule[%d]: %w", owner.AppNameID, index, err)
			}
			resolved.Domains[owner.AppNameID][index] = addresses
		}
	}
	return nil
}

func resolveRule(meta schema.DNSMeta, names []string, lookup Lookup) ([]string, error) {
	seen := map[string]bool{}
	for _, name := range names {
		if !domain.ValidDomain(name) {
			return nil, fmt.Errorf("invalid domain %q", name)
		}
		addresses, err := lookup(meta, name)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", name, err)
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("resolve %s: empty answer", name)
		}
		for _, address := range addresses {
			if !address.IsValid() || address.Zone() != "" || address.Is4In6() {
				return nil, fmt.Errorf("resolve %s: invalid address", name)
			}
			seen[address.String()] = true
		}
	}
	result := make([]string, 0, len(seen))
	for address := range seen {
		result = append(result, address)
	}
	sort.Strings(result)
	return result, nil
}

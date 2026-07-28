package netenforce

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// LookupFunc resolves a hostname to its addresses. It exists so the resolution step can be
// driven in tests without a network, and so a future mechanism (a resolver inside the netns,
// a live set refreshed while the app runs) can be swapped in behind the same call.
type LookupFunc func(host string) ([]net.IP, error)

// resolveDomains returns cfg with every egress list's Domains folded into that list's
// IPv4CIDR and IPv6CIDR as host routes, so the ruleset renderer never has to know that
// names were involved. The input is not modified.
//
// This is where the guarantee is actually set, so it is worth stating exactly. What comes
// out is an ADDRESS allowlist, taken at this moment. It stops an app from reaching anywhere
// those names did not resolve to, and it does not stop the app from reaching one of those
// addresses without asking DNS, nor does it notice a hostname on the wire. It is also a
// snapshot: nothing refreshes it while the app runs, so a domain whose addresses rotate
// drifts out of the set and the app loses access until it is restarted.
//
// A domain that does not resolve fails the launch rather than silently contributing nothing.
// The alternative is an app that starts, looks healthy, and cannot reach the one host it
// exists to talk to - with the reason visible nowhere. Failing closed is also failing loud.
func resolveDomains(cfg schema.AppConfig, lookup LookupFunc) (schema.AppConfig, error) {
	if !hasDomains(cfg) {
		return cfg, nil
	}
	if lookup == nil {
		lookup = net.LookupIP
	}
	lists := make([]schema.NetworkList, len(cfg.NetworkMeta.NetworkLists))
	copy(lists, cfg.NetworkMeta.NetworkLists)
	for index, netList := range lists {
		if len(netList.Domains) == 0 {
			continue
		}
		v4, v6, err := resolveList(netList.Domains, lookup)
		if err != nil {
			return schema.AppConfig{}, fmt.Errorf("%s: NetworkLists[%d]: %w", cfg.AppNameID, index, err)
		}
		// Appended to whatever the list already allows by address: a list may name both, and
		// the domains are simply more destinations under the same Ports.
		lists[index].IPv4CIDR = append(append([]string{}, netList.IPv4CIDR...), v4...)
		lists[index].IPv6CIDR = append(append([]string{}, netList.IPv6CIDR...), v6...)
	}
	cfg.NetworkMeta.NetworkLists = lists
	return cfg, nil
}

// hasDomains reports whether any list allows by name, so the common case pays nothing - no
// copying, and above all no DNS lookups on a launch that never asked for any.
func hasDomains(cfg schema.AppConfig) bool {
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if len(netList.Domains) > 0 {
			return true
		}
	}
	return false
}

// resolveList resolves one list's domains into host routes (/32 and /128), deduplicated and
// sorted so the same config renders the same ruleset twice running - a ruleset that reordered
// itself between launches would be impossible to diff.
func resolveList(domains []string, lookup LookupFunc) (v4, v6 []string, err error) {
	seen4, seen6 := map[string]bool{}, map[string]bool{}
	for _, domain := range domains {
		host := strings.TrimSpace(domain)
		if host == "" {
			continue
		}
		addresses, lerr := lookup(host)
		if lerr != nil {
			return nil, nil, fmt.Errorf("resolving %q: %w", host, lerr)
		}
		if len(addresses) == 0 {
			return nil, nil, fmt.Errorf("resolving %q: no addresses", host)
		}
		for _, address := range addresses {
			if four := address.To4(); four != nil {
				seen4[four.String()+"/32"] = true
				continue
			}
			seen6[address.String()+"/128"] = true
		}
	}
	v4, v6 = keys(seen4), keys(seen6)
	if len(v4) == 0 && len(v6) == 0 {
		// Every name resolved, and to nothing usable. Rendering an empty set would leave a
		// list that reads as an allowance and permits nothing, which is the shape of bug this
		// codebase keeps refusing elsewhere.
		return nil, nil, fmt.Errorf("resolving %v: nothing usable came back", domains)
	}
	return v4, v6, nil
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

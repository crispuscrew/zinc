package netenforce

import (
	"errors"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

// fakeLookup answers from a table, so resolution is exercised without a network and without
// depending on what any real name resolves to today.
func fakeLookup(table map[string][]string) LookupFunc {
	return func(host string) ([]net.IP, error) {
		addresses, ok := table[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		out := make([]net.IP, 0, len(addresses))
		for _, address := range addresses {
			out = append(out, net.ParseIP(address))
		}
		return out, nil
	}
}

func domainApp(domains ...string) schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     "app",
		ImageMeta:     schema.ImageMeta{Image: "localhost/app:local"},
		NetworkMeta: schema.NetworkMeta{NetworkLists: []schema.NetworkList{{
			Domains: domains,
			Ports:   []int{443},
		}}},
	}
}

// A domain becomes host routes on the list that named it, under that list's ports. The
// renderer never learns names were involved, which is what keeps it pure.
func TestResolveDomains_BecomeHostRoutes(t *testing.T) {
	cfg, err := resolveDomains(domainApp("api.example.com"), fakeLookup(map[string][]string{
		"api.example.com": {"203.0.113.7", "2001:db8::7"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	list := cfg.NetworkMeta.NetworkLists[0]
	if want := []string{"203.0.113.7/32"}; !slices.Equal(list.IPv4CIDR, want) {
		t.Errorf("IPv4CIDR = %v, want %v", list.IPv4CIDR, want)
	}
	if want := []string{"2001:db8::7/128"}; !slices.Equal(list.IPv6CIDR, want) {
		t.Errorf("IPv6CIDR = %v, want %v", list.IPv6CIDR, want)
	}

	ruleset := NFTRuleset(cfg)
	if !strings.Contains(ruleset, "ip daddr { 203.0.113.7/32 } tcp dport { 443 } accept") {
		t.Errorf("the resolved address should be an ordinary accept rule:\n%s", ruleset)
	}
}

// Addresses a list already allows by number are kept: a list may name both, and the domains
// are more destinations under the same ports rather than a replacement for them.
func TestResolveDomains_AppendToStatedCIDRs(t *testing.T) {
	cfg := domainApp("api.example.com")
	cfg.NetworkMeta.NetworkLists[0].IPv4CIDR = []string{"198.51.100.0/24"}
	resolved, err := resolveDomains(cfg, fakeLookup(map[string][]string{
		"api.example.com": {"203.0.113.7"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"198.51.100.0/24", "203.0.113.7/32"}
	if got := resolved.NetworkMeta.NetworkLists[0].IPv4CIDR; !slices.Equal(got, want) {
		t.Errorf("IPv4CIDR = %v, want %v", got, want)
	}
}

// The input config must not be modified: the caller still holds it, and a launch that
// rewrote the app's own definition in memory would make the second read differ from the
// first for no visible reason.
func TestResolveDomains_DoesNotMutateInput(t *testing.T) {
	cfg := domainApp("api.example.com")
	if _, err := resolveDomains(cfg, fakeLookup(map[string][]string{
		"api.example.com": {"203.0.113.7"},
	})); err != nil {
		t.Fatal(err)
	}
	if got := cfg.NetworkMeta.NetworkLists[0].IPv4CIDR; len(got) != 0 {
		t.Errorf("the caller's config was modified: IPv4CIDR = %v", got)
	}
}

// Two domains behind one address, and one domain behind several: the set is deduplicated and
// sorted, so the same config renders the same ruleset twice running.
func TestResolveDomains_DedupedAndSorted(t *testing.T) {
	cfg, err := resolveDomains(domainApp("one.example.com", "two.example.com"), fakeLookup(map[string][]string{
		"one.example.com": {"203.0.113.9", "203.0.113.7"},
		"two.example.com": {"203.0.113.7"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"203.0.113.7/32", "203.0.113.9/32"}
	if got := cfg.NetworkMeta.NetworkLists[0].IPv4CIDR; !slices.Equal(got, want) {
		t.Errorf("IPv4CIDR = %v, want %v", got, want)
	}
}

// A name that does not resolve fails the launch. The alternative is an app that starts,
// looks healthy, and cannot reach the one host it exists to talk to, with the reason visible
// nowhere - failing closed here is also failing loud.
func TestResolveDomains_UnresolvableFailsTheLaunch(t *testing.T) {
	_, err := Enforcer{Lookup: fakeLookup(nil)}.Prepare(domainApp("api.example.com"), options.HostOptions{})
	if err == nil {
		t.Fatal("a domain that does not resolve must fail the launch")
	}
	if !strings.Contains(err.Error(), "api.example.com") {
		t.Errorf("the failure should name the domain, got %v", err)
	}
}

// A name that resolves to nothing usable is the same case wearing a different hat: rendering
// an empty set would leave a list that reads as an allowance and permits nothing.
func TestResolveDomains_EmptyAnswerFailsTheLaunch(t *testing.T) {
	lookup := LookupFunc(func(string) ([]net.IP, error) { return nil, nil })
	if _, err := resolveDomains(domainApp("api.example.com"), lookup); err == nil {
		t.Fatal("a name that resolves to no addresses must fail rather than allow nothing quietly")
	}
}

// An app that names no domains must not resolve anything at all - the common launch pays no
// DNS lookups, and a resolver that is down cannot stop it starting.
func TestResolveDomains_NoDomainsNoLookups(t *testing.T) {
	cfg := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     "app",
		ImageMeta:     schema.ImageMeta{Image: "localhost/app:local"},
		NetworkMeta: schema.NetworkMeta{NetworkLists: []schema.NetworkList{{
			IPv4CIDR: []string{"1.1.1.1/32"}, Ports: []int{443},
		}}},
	}
	lookup := LookupFunc(func(host string) ([]net.IP, error) {
		t.Fatalf("no lookup should happen, got one for %q", host)
		return nil, nil
	})
	if _, err := resolveDomains(cfg, lookup); err != nil {
		t.Fatal(err)
	}
}

// Only the list that named the domains gains addresses; a second list is untouched.
func TestResolveDomains_PerList(t *testing.T) {
	cfg := domainApp("api.example.com")
	cfg.NetworkMeta.NetworkLists = append(cfg.NetworkMeta.NetworkLists,
		schema.NetworkList{IPv4CIDR: []string{"192.0.2.0/24"}, Ports: []int{80}})
	resolved, err := resolveDomains(cfg, fakeLookup(map[string][]string{
		"api.example.com": {"203.0.113.7"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.NetworkMeta.NetworkLists[1].IPv4CIDR; !slices.Equal(got, []string{"192.0.2.0/24"}) {
		t.Errorf("the second list should be untouched, got %v", got)
	}
}

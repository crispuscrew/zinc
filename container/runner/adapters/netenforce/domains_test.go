package netenforce

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func TestDomainsUseConfiguredResolverWithoutMutation(t *testing.T) {
	cfg, manifest := configured()
	cfg.NetworkMeta.RulesByPriority[0].Domains = []string{"example.org"}
	cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSQUIC, Endpoint: "1.1.1.1"}}
	manifest.Policy = cfg.NetworkMeta
	manifest.DNSProxyAddresses, manifest.DNSConfigDigest = []string{"10.0.0.1"}, provision.DNSDigest(cfg.NetworkMeta.DNS)
	readyProxy(t, &manifest)
	enforcer := adapter(manifest)
	if _, err := enforcer.Prepare(cfg, options.HostOptions{}); err == nil {
		t.Fatal("nil resolver fell back to host")
	}
	enforcer.Lookup = func(meta schema.DNSMeta, host string) ([]netip.Addr, error) {
		if meta.ResolversByPriority[0].Protocol != schema.DNSQUIC || host != "example.org" {
			t.Fatal("wrong resolution authority")
		}
		return []netip.Addr{netip.MustParseAddr("1.1.1.2"), netip.MustParseAddr("1.1.1.2")}, nil
	}
	steps, err := enforcer.Prepare(cfg, options.HostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(steps[0].Stdin, "ip daddr { 1.1.1.2/32 }") || strings.Contains(steps[0].Stdin, "1.1.1.2/32, 1.1.1.2/32") {
		t.Fatal("domain resolution not deduplicated")
	}
	if len(cfg.NetworkMeta.RulesByPriority[0].To.Filter.IPv4CIDR) != 0 {
		t.Fatal("input mutated")
	}
	for _, failure := range []error{nil, errors.New("upstream failed")} {
		enforcer.Lookup = func(schema.DNSMeta, string) ([]netip.Addr, error) { return nil, failure }
		if _, err := enforcer.Prepare(cfg, options.HostOptions{}); err == nil {
			t.Fatal("empty/failed lookup allowed launch")
		}
	}
}

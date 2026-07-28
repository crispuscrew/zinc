package validate

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// baseCfg is a minimal AppConfig that passes Validate with no network lists, so a test
// can inject one NetworkList and attribute any error/warning to it alone.
func baseCfg() schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     "app",
		ImageMeta:     schema.ImageMeta{Image: "localhost/app:local"},
	}
}

func withList(list schema.NetworkList) schema.AppConfig {
	cfg := baseCfg()
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{list}
	return cfg
}

// An egress list carrying ports but no CIDR emits nothing in the output chain, silently
// reverting to the default policy (the DNS-block footgun). It must be rejected.
func TestEgressPortsWithoutCIDRRejected(t *testing.T) {
	err := Validate(withList(schema.NetworkList{Blacklist: true, Ports: []int{53, 853}}))
	if err == nil {
		t.Fatal("egress ports with no CIDR: want error, got nil")
	}
	if !strings.Contains(err.Error(), "needs destinations") {
		t.Fatalf("want a missing-destinations error, got: %v", err)
	}
}

// The same list with an explicit all-destinations CIDR is the correct DNS-block form.
func TestEgressPortsWithCIDROK(t *testing.T) {
	err := Validate(withList(schema.NetworkList{
		Blacklist: true, Ports: []int{53, 853}, IPv4CIDR: []string{"0.0.0.0/0"},
	}))
	if err != nil {
		t.Fatalf("egress ports with a CIDR: want nil, got: %v", err)
	}
}

// Ingress ports need no CIDR - an empty source allowlist means "any source".
func TestIngressPortsWithoutCIDROK(t *testing.T) {
	err := Validate(withList(schema.NetworkList{Ingress: true, Ports: []int{5432}}))
	if err != nil {
		t.Fatalf("ingress ports with no CIDR: want nil, got: %v", err)
	}
}

func TestIngressWarnings(t *testing.T) {
	cases := []struct {
		name string
		list schema.NetworkList
		want string
	}{
		{
			name: "self exposes ports",
			list: schema.NetworkList{Ingress: true, Ports: []int{5432}},
			want: "ingress exposes port(s) 5432 to apps that join",
		},
		{
			name: "host is LAN",
			list: schema.NetworkList{Ingress: true, Host: true, Interface: "eth0", Ports: []int{80}},
			want: "the LAN via eth0",
		},
		{
			name: "host no interface names all",
			list: schema.NetworkList{Ingress: true, Host: true, Ports: []int{80}},
			want: "the LAN via all host interfaces",
		},
		{
			name: "blacklist exposes all",
			list: schema.NetworkList{Ingress: true, Blacklist: true},
			want: "exposes ALL inbound ports",
		},
		{
			name: "no ports likely a mistake",
			list: schema.NetworkList{Ingress: true},
			want: "did you forget Ports?",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warns := Warnings(withList(tc.list))
			if len(warns) != 1 || !strings.Contains(warns[0], tc.want) {
				t.Fatalf("want a warning containing %q, got: %v", tc.want, warns)
			}
		})
	}
}

// An egress list is not treated as ingress: an empty egress blacklist warns about
// allow-all, and it must not produce an ingress-exposure warning.
func TestEgressEmptyBlacklistWarns(t *testing.T) {
	warns := Warnings(withList(schema.NetworkList{Blacklist: true}))
	if len(warns) != 1 || !strings.Contains(warns[0], "egress blacklist") {
		t.Fatalf("want an egress allow-all warning, got: %v", warns)
	}
}

// A domain is handed to a resolver, so it must be a plain hostname: a URL, a port, or a
// trailing dot would either fail to resolve or resolve to something other than what the
// author read when they wrote it.
func TestDomains_MustBePlainHostnames(t *testing.T) {
	for _, bad := range []string{
		"https://example.com", "example.com:443", "example.com/path", "example.com.",
		"Example.com", "", "-example.com",
	} {
		err := Validate(withList(schema.NetworkList{Domains: []string{bad}, Ports: []int{443}}))
		if err == nil || !strings.Contains(err.Error(), "Domains") {
			t.Errorf("Domains %q: want a Domains error, got: %v", bad, err)
		}
	}
	for _, good := range []string{"example.com", "api.example.com", "a-b.example.co.uk", "localhost"} {
		if err := Validate(withList(schema.NetworkList{Domains: []string{good}, Ports: []int{443}})); err != nil {
			t.Errorf("Domains %q should be accepted, got: %v", good, err)
		}
	}
}

// The refusals are about what the enforcement can actually deliver, not tidiness. Allowing
// by name is resolving a name to addresses and permitting those; every shape below would
// read as something else.
func TestDomains_RefusedWhereTheyWouldMislead(t *testing.T) {
	for _, testCase := range []struct {
		name string
		list schema.NetworkList
		want string
	}{
		{
			// An incoming packet carries an address, not a name; resolving the domain would
			// admit whoever holds that address rather than whoever owns the name.
			name: "ingress",
			list: schema.NetworkList{Ingress: true, Domains: []string{"example.com"}, Ports: []int{443}},
			want: "only an egress list can allow by name",
		},
		{
			// A domain blacklist would have to be every address the name does NOT resolve to.
			name: "blacklist",
			list: schema.NetworkList{Blacklist: true, Domains: []string{"example.com"}, Ports: []int{443}},
			want: "cannot be used on a blacklist",
		},
		{
			// A sibling link is gated by interface, so an address set on it enforces nothing.
			name: "link",
			list: schema.NetworkList{AppName: "db", Domains: []string{"example.com"}, Ports: []int{443}},
			want: "no meaning on a sibling link",
		},
	} {
		err := Validate(withList(testCase.list))
		if err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Errorf("%s: want %q, got: %v", testCase.name, testCase.want, err)
		}
	}
}

// Domains are destinations, so ports alongside them are anchored to something - the rule
// that rejects ports with no destination must not fire.
func TestDomains_SatisfyThePortsNeedDestinationsRule(t *testing.T) {
	if err := Validate(withList(schema.NetworkList{Domains: []string{"example.com"}, Ports: []int{443}})); err != nil {
		t.Fatalf("domains are destinations for a port rule, got: %v", err)
	}
}

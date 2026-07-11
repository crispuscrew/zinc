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
	if !strings.Contains(err.Error(), "destination CIDRs") {
		t.Fatalf("want a destination-CIDR error, got: %v", err)
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

// Ingress ports need no CIDR — an empty source allowlist means "any source".
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

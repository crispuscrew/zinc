package netenforce

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/hyprzinc/core/domain"
)

func pastaApp() domain.AppConfig {
	return domain.AppConfig{
		SchemaVersion: domain.SchemaVersion,
		App:           domain.App{Name: "browser", Image: "docker.io/library/firefox@sha256:abc"},
		Display:       domain.Display{Wayland: domain.WaylandPassthrough},
		Network: domain.Network{
			Mode:     domain.NetworkHost,
			IPv4CIDR: []string{"1.1.1.1/32", "9.9.9.9/32"},
			Ports:    []int{443, 80},
			BlockDNS: true,
		},
		Theme: domain.Theme{Mode: domain.ThemeNone},
	}
}

func TestNFTRuleset_Allowlist(t *testing.T) {
	rules := NFTRuleset(pastaApp())
	for _, want := range []string{
		"policy drop;",
		`oif "lo" accept`,
		"ct state established,related accept",
		"udp dport { 53, 853 } drop",
		"tcp dport { 53, 853 } drop",
		"ip daddr { 1.1.1.1/32, 9.9.9.9/32 } tcp dport { 443, 80 } accept",
		"ip daddr { 1.1.1.1/32, 9.9.9.9/32 } udp dport { 443, 80 } accept",
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("ruleset missing %q\n---\n%s", want, rules)
		}
	}
	if strings.Contains(rules, "ip6 daddr") {
		t.Errorf("unexpected ip6 allow rule (ipv6 should be blocked):\n%s", rules)
	}
}

func TestNFTRuleset_NoDNSBlock(t *testing.T) {
	cfg := pastaApp()
	cfg.Network.BlockDNS = false
	if rules := NFTRuleset(cfg); strings.Contains(rules, "dport { 53, 853 } drop") {
		t.Errorf("block_dns off → no explicit 53/853 drop expected:\n%s", rules)
	}
}

func TestNFTRuleset_CIDRWithoutPorts(t *testing.T) {
	cfg := pastaApp()
	cfg.Network.Ports = nil
	cfg.Network.BlockDNS = false
	rules := NFTRuleset(cfg)
	if !strings.Contains(rules, "ip daddr { 1.1.1.1/32, 9.9.9.9/32 } accept") {
		t.Errorf("no ports → all-ports accept to CIDRs expected:\n%s", rules)
	}
	if strings.Contains(rules, "dport") {
		t.Errorf("no ports and no block_dns → no dport rules expected:\n%s", rules)
	}
}

func TestNFTRuleset_IPv6(t *testing.T) {
	cfg := pastaApp()
	cfg.Network.IPv6CIDR = []string{"2001:db8::/32"}
	if rules := NFTRuleset(cfg); !strings.Contains(rules, "ip6 daddr { 2001:db8::/32 } tcp dport { 443, 80 } accept") {
		t.Errorf("ipv6 allow rule missing:\n%s", rules)
	}
}

// The pasta enforcer attaches the app to its pod and prepares the netns with two
// steps — pod create (pasta netns) then nft lock — before the app ever runs, so
// there is no unfiltered-egress window (§5.3).
func TestPasta_RunFlagsAndPrepare(t *testing.T) {
	cfg := pastaApp()
	pod := PodName(cfg.App.Name)

	if got := (Pasta{}).RunFlags(cfg); !slices.Equal(got, []string{"--pod", pod}) {
		t.Fatalf("pasta RunFlags should join the pod, got %v", got)
	}

	steps := Pasta{}.Prepare(cfg, domain.HostOptions{})
	if len(steps) != 2 {
		t.Fatalf("pasta prepare should be two steps (pod create, nft lock), got %d", len(steps))
	}
	// 1. pod create with pasta networking
	assertContainsSeq(t, steps[0].Args, "pod", "create")
	assertContainsSeq(t, steps[0].Args, "--name", pod)
	assertContainsSeq(t, steps[0].Args, "--network", "pasta")
	// 2. nft lock-down: only NET_ADMIN, joined to the pod, ruleset on stdin, local-only helper
	assertContainsSeq(t, steps[1].Args, "--pod", pod)
	assertContainsSeq(t, steps[1].Args, "--cap-add", "NET_ADMIN")
	assertContainsSeq(t, steps[1].Args, "--pull", "never")
	if steps[1].Stdin != NFTRuleset(cfg) {
		t.Fatal("nft step must carry the ruleset on stdin")
	}
	if tail := steps[1].Args[len(steps[1].Args)-3:]; !slices.Equal(tail, []string{"nft", "-f", "-"}) {
		t.Fatalf("nft step should end with `nft -f -`, got %v", tail)
	}
}

func TestPasta_NetfilterImageOverride(t *testing.T) {
	steps := Pasta{}.Prepare(pastaApp(), domain.HostOptions{NetfilterImage: "my/nft:local"})
	if !slices.Contains(steps[1].Args, "my/nft:local") {
		t.Fatalf("nft step should use the override image, got %v", steps[1].Args)
	}
}

func TestEnforcers_Teardown(t *testing.T) {
	pasta := pastaApp()
	if got, want := (Pasta{}).Teardown(pasta), []string{"pod", "rm", "-f", PodName(pasta.App.Name)}; !slices.Equal(got, want) {
		t.Fatalf("pasta teardown: got %v want %v", got, want)
	}
	if got, want := (None{}).Teardown(pasta), []string{"stop", pasta.App.Name}; !slices.Equal(got, want) {
		t.Fatalf("none teardown: got %v want %v", got, want)
	}
}

func TestBasicEnforcers_RunFlags(t *testing.T) {
	if got := (None{}).RunFlags(domain.AppConfig{}); !slices.Equal(got, []string{"--network", "none"}) {
		t.Fatalf("none RunFlags: %v", got)
	}
	cfg := domain.AppConfig{Network: domain.Network{Mode: domain.NetworkContainer, Target: "vpn-container"}}
	if got := (Container{}).RunFlags(cfg); !slices.Equal(got, []string{"--network", "container:vpn-container"}) {
		t.Fatalf("container RunFlags: %v", got)
	}
	if (None{}).Prepare(cfg, domain.HostOptions{}) != nil || (Container{}).Prepare(cfg, domain.HostOptions{}) != nil {
		t.Fatal("none/container enforcers must have no prepare steps")
	}
}

// assertContainsSeq checks that first and second appear adjacent and in order.
func assertContainsSeq(t *testing.T, args []string, first, second string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return
		}
	}
	t.Fatalf("expected adjacent %q %q in %v", first, second, args)
}

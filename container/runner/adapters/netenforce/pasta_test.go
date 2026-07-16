package netenforce

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

// pastaApp is a filtered app: one self-scoped whitelist list (default-drop egress,
// allow the listed CIDRs/ports).
func pastaApp() schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     "browser",
		ImageMeta:     schema.ImageMeta{Image: "docker.io/library/firefox@sha256:abc"},
		NetworkMeta: schema.NetworkMeta{NetworkLists: []schema.NetworkList{{
			IPv4CIDR: []string{"1.1.1.1/32", "9.9.9.9/32"},
			Ports:    []int{443, 80},
		}}},
	}
}

func TestNFTRuleset_Allowlist(t *testing.T) {
	rules := NFTRuleset(pastaApp())
	for _, want := range []string{
		"policy drop;",
		`oif "lo" accept`,
		"ct state established,related accept",
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

// An all-blacklist app is allow-all-except: the chain default is accept and only the
// listed CIDRs are dropped.
func TestNFTRuleset_BlacklistIsAllowAllExcept(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{
		Blacklist: true,
		IPv4CIDR:  []string{"10.0.0.0/8"},
	}}
	rules := NFTRuleset(cfg)
	if !strings.Contains(rules, "policy accept;") {
		t.Errorf("all-blacklist app should default-accept:\n%s", rules)
	}
	if !strings.Contains(rules, "ip daddr { 10.0.0.0/8 } drop") {
		t.Errorf("blacklist entry should drop the listed CIDR:\n%s", rules)
	}
}

// DNS blocking is no longer a dedicated flag: it is a normal blacklist rule for ports
// 53/853 scoped to all destinations. validate rejects a port rule with no CIDR, so this
// is the canonical form (allow-all-except-DNS: chain default accept, DNS dropped).
func TestNFTRuleset_DNSBlockViaBlacklist(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{
		Blacklist: true,
		IPv4CIDR:  []string{"0.0.0.0/0"},
		IPv6CIDR:  []string{"::/0"},
		Ports:     []int{53, 853},
	}}
	rules := NFTRuleset(cfg)
	for _, want := range []string{
		"policy accept;",
		"ip daddr { 0.0.0.0/0 } tcp dport { 53, 853 } drop",
		"ip daddr { 0.0.0.0/0 } udp dport { 53, 853 } drop",
		"ip6 daddr { ::/0 } tcp dport { 53, 853 } drop",
		"ip6 daddr { ::/0 } udp dport { 53, 853 } drop",
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("DNS-block ruleset missing %q\n---\n%s", want, rules)
		}
	}
}

func TestNFTRuleset_CIDRWithoutPorts(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists[0].Ports = nil
	rules := NFTRuleset(cfg)
	if !strings.Contains(rules, "ip daddr { 1.1.1.1/32, 9.9.9.9/32 } accept") {
		t.Errorf("no ports → all-ports accept to CIDRs expected:\n%s", rules)
	}
	if strings.Contains(rules, "dport") {
		t.Errorf("no ports and no DNS-block: no dport rules expected:\n%s", rules)
	}
}

func TestNFTRuleset_IPv6(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists[0].IPv6CIDR = []string{"2001:db8::/32"}
	if rules := NFTRuleset(cfg); !strings.Contains(rules, "ip6 daddr { 2001:db8::/32 } tcp dport { 443, 80 } accept") {
		t.Errorf("ipv6 allow rule missing:\n%s", rules)
	}
}

// An egress-only app has no input base chain at all - ingress stays closed by omission.
func TestNFTRuleset_NoInputChainForEgressOnly(t *testing.T) {
	if rules := NFTRuleset(pastaApp()); strings.Contains(rules, "chain input") {
		t.Errorf("egress-only app should have no input chain:\n%s", rules)
	}
}

// A tier-3 (LAN) publish builds a default-drop input chain that accepts the published
// ports only from the source CIDRs; its output chain still default-drops egress.
func TestNFTRuleset_IngressInputChain(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{
		Ingress:  true,
		Host:     true,
		IPv4CIDR: []string{"192.168.1.0/24"},
		Ports:    []int{80, 443},
	}}
	rules := NFTRuleset(cfg)
	for _, want := range []string{
		"chain input {",
		"hook input priority 0; policy drop;",
		`iif "lo" accept`,
		"ip saddr { 192.168.1.0/24 } tcp dport { 80, 443 } accept",
		"ip saddr { 192.168.1.0/24 } udp dport { 80, 443 } accept",
		"hook output priority 0; policy drop;", // pure publisher: no egress
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("input-chain ruleset missing %q\n---\n%s", want, rules)
		}
	}
}

// With no source CIDR an ingress list opens the ports to any source (no saddr match).
func TestNFTRuleset_IngressAnySource(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{Ingress: true, Host: true, Ports: []int{8080}}}
	rules := NFTRuleset(cfg)
	if !strings.Contains(rules, "tcp dport { 8080 } accept") {
		t.Errorf("no CIDR should accept the port from any source:\n%s", rules)
	}
	if strings.Contains(rules, "saddr") {
		t.Errorf("no CIDR should emit no saddr match:\n%s", rules)
	}
}

// A tier-3 list forwards its ports onto the pod (tcp and udp), with no host-port remap.
func TestPodCreate_PublishesTier3Ports(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{Ingress: true, Host: true, Ports: []int{80}}}
	steps := Enforcer{}.Prepare(cfg, options.HostOptions{})
	create := steps[0].Args
	assertContainsSeq(t, create, "-p", "80:80/tcp")
	assertContainsSeq(t, create, "-p", "80:80/udp")
}

// A tier-2 producer (self-scoped ingress) publishes nothing to the host - no `-p` in any
// prepare step (it is reachable only over its private link).
func TestPodCreate_Tier2PublishesNothing(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{Ingress: true, Ports: []int{5432}}}
	for _, step := range (Enforcer{}).Prepare(cfg, options.HostOptions{}) {
		if slices.Contains(step.Args, "-p") {
			t.Errorf("tier-2 producer must not publish to the host:\n%v", step.Args)
		}
	}
}

// A tier-2 producer's pod attaches only to its own private link on a fixed interface -
// no pasta - after the bridge is created idempotently as internal.
func TestTier2_ProducerPrepare(t *testing.T) {
	cfg := pastaApp()
	cfg.AppNameID = "db"
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{Ingress: true, Ports: []int{5432}}}
	steps := Enforcer{}.Prepare(cfg, options.HostOptions{})
	assertContainsSeq(t, steps[0].Args, "network", "create")
	for _, want := range []string{"--ignore", "--internal", "zinc-link-db"} {
		if !slices.Contains(steps[0].Args, want) {
			t.Fatalf("link create missing %q, got %v", want, steps[0].Args)
		}
	}
	if !slices.Contains(steps[1].Args, "zinc-link-db:interface_name=zlink0,alias=db") {
		t.Fatalf("pod should attach to its link on zlink0 with alias=db, got %v", steps[1].Args)
	}
	if slices.Contains(steps[1].Args, "pasta") {
		t.Fatalf("a tier-2 pod must not use pasta, got %v", steps[1].Args)
	}
}

// A tier-2 consumer attaches to the producer's link and reaches it over that interface;
// it accepts nothing new inbound (it publishes no ports).
func TestTier2_ConsumerRuleset(t *testing.T) {
	cfg := pastaApp()
	cfg.AppNameID = "client"
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{AppName: "db"}}
	if !slices.Contains(podCreateArgs(cfg, PodName("client")), "zinc-link-db:interface_name=zlink0,alias=client") {
		t.Fatalf("consumer should attach to the producer's link with its own alias")
	}
	rules := NFTRuleset(cfg)
	if !strings.Contains(rules, `oifname "zlink0" accept`) {
		t.Errorf("consumer should reach the producer over the link:\n%s", rules)
	}
	if strings.Contains(rules, "dport") {
		t.Errorf("a consumer publishes nothing, so no dport accepts expected:\n%s", rules)
	}
}

// A tier-2 producer's ruleset is interface-gated: its published ports are accepted inbound
// only on its own link interface, link traffic is permitted out, both chains default-drop,
// and there are no address rules.
func TestTier2_ProducerRuleset(t *testing.T) {
	cfg := pastaApp()
	cfg.AppNameID = "db"
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{Ingress: true, Ports: []int{5432}}}
	rules := NFTRuleset(cfg)
	for _, want := range []string{
		"hook input priority 0; policy drop;",
		"hook output priority 0; policy drop;",
		`iifname "zlink0" tcp dport { 5432 } accept`,
		`iifname "zlink0" udp dport { 5432 } accept`,
		`oifname "zlink0" accept`,
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("producer link ruleset missing %q\n---\n%s", want, rules)
		}
	}
	if strings.Contains(rules, "daddr") || strings.Contains(rules, "saddr") {
		t.Errorf("a tier-2 ruleset must be interface-gated, not address-gated:\n%s", rules)
	}
}

// The enforcer attaches a filtered app to its pod and prepares the netns with two
// steps - pod create (pasta netns) then nft lock - before the app ever runs, so there
// is no unfiltered-egress window (section 5.3).
func TestEnforcer_RunFlagsAndPrepare(t *testing.T) {
	cfg := pastaApp()
	pod := PodName(cfg.AppNameID)

	if got := (Enforcer{}).RunFlags(cfg); !slices.Equal(got, []string{"--pod", pod}) {
		t.Fatalf("filtered RunFlags should join the pod, got %v", got)
	}

	steps := Enforcer{}.Prepare(cfg, options.HostOptions{})
	if len(steps) != 2 {
		t.Fatalf("filtered prepare should be two steps (pod create, nft lock), got %d", len(steps))
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

func TestEnforcer_NetfilterImageOverride(t *testing.T) {
	steps := Enforcer{}.Prepare(pastaApp(), options.HostOptions{NetfilterImage: "my/nft:local"})
	if !slices.Contains(steps[1].Args, "my/nft:local") {
		t.Fatalf("nft step should use the override image, got %v", steps[1].Args)
	}
}

// An app with no NetworkLists is unfiltered: --network none, nothing to prepare, and a
// plain container stop on teardown.
func TestEnforcer_Unfiltered(t *testing.T) {
	cfg := schema.AppConfig{AppNameID: "solo"}
	if got := (Enforcer{}).RunFlags(cfg); !slices.Equal(got, []string{"--network", "none"}) {
		t.Fatalf("unfiltered RunFlags: %v", got)
	}
	if steps := (Enforcer{}).Prepare(cfg, options.HostOptions{}); steps != nil {
		t.Fatalf("unfiltered app has nothing to prepare, got %v", steps)
	}
	if got, want := (Enforcer{}).Teardown(cfg), []string{"stop", "solo"}; !slices.Equal(got, want) {
		t.Fatalf("unfiltered teardown: got %v want %v", got, want)
	}
}

func TestEnforcer_FilteredTeardown(t *testing.T) {
	cfg := pastaApp()
	if got, want := (Enforcer{}).Teardown(cfg), []string{"pod", "rm", "-f", PodName(cfg.AppNameID)}; !slices.Equal(got, want) {
		t.Fatalf("filtered teardown: got %v want %v", got, want)
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

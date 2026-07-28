package netenforce

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

// prepare runs the enforcer's pre-steps and fails the test if they could not be built. Most
// of these cases resolve no domains, so the error path is exercised on its own in
// domains_test.go rather than restated here.
func prepare(t *testing.T, cfg schema.AppConfig, opt options.HostOptions) []ports.Command {
	t.Helper()
	steps, err := Enforcer{}.Prepare(cfg, opt)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return steps
}

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
	steps := prepare(t, cfg, options.HostOptions{})
	create := steps[0].Args
	assertContainsSeq(t, create, "-p", "80:80/tcp")
	assertContainsSeq(t, create, "-p", "80:80/udp")
}

// A tier-2 producer (self-scoped ingress) publishes nothing to the host - no `-p` in any
// prepare step (it is reachable only over its private link).
func TestPodCreate_Tier2PublishesNothing(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{Ingress: true, Ports: []int{5432}}}
	for _, step := range prepare(t, cfg, options.HostOptions{}) {
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
	steps := prepare(t, cfg, options.HostOptions{})
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

	steps := prepare(t, cfg, options.HostOptions{})
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
	steps := prepare(t, pastaApp(), options.HostOptions{NetfilterImage: "my/nft:local"})
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
	if steps := prepare(t, cfg, options.HostOptions{}); steps != nil {
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

// gatewayApp is the shape the whole vpn-routing feature is for: an app that serves its
// siblings over a private link AND reaches the outside itself. Refused until now.
func gatewayApp() schema.AppConfig {
	cfg := pastaApp()
	cfg.AppNameID = "vpn"
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{
		{Ingress: true, Ports: []int{1080}},                         // serves siblings on its link
		{IPv4CIDR: []string{"203.0.113.7/32"}, Ports: []int{51820}}, // reaches its tunnel endpoint
	}
	return cfg
}

// One app, gated both ways at once: the private bridge by interface, the outside world by
// address. Whichever ruleset ran before ignored the other kind of list outright, which is
// why the combination was rejected rather than rendered.
func TestNFTRuleset_LinkAndEgressTogether(t *testing.T) {
	got := NFTRuleset(gatewayApp())

	for _, want := range []string{
		`oifname "zlink0" accept`,                                // the link, by interface
		`ip daddr { 203.0.113.7/32 } tcp dport { 51820 } accept`, // the tunnel, by address
		`iifname "zlink0" tcp dport { 1080 } accept`,             // siblings reaching its published port
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ruleset missing %q:\n%s", want, got)
		}
	}
}

// The subtle one. A link list is structurally a whitelist, so folding it into the policy
// decision would flip an app that pairs a link with an all-blacklist egress to default-drop
// and silently deny everything the blacklist meant to leave open. Policy comes from the
// non-link lists alone.
func TestNFTRuleset_LinkDoesNotFlipABlacklistPolicy(t *testing.T) {
	cfg := pastaApp()
	cfg.AppNameID = "client"
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{
		{AppName: "vpn"}, // a link: always a whitelist
		{Blacklist: true, IPv4CIDR: []string{"0.0.0.0/0"}, Ports: []int{53}}, // all but DNS
	}
	got := NFTRuleset(cfg)
	if !strings.Contains(got, "hook output priority 0; policy accept;") {
		t.Errorf("an all-blacklist egress must stay default-accept beside a link:\n%s", got)
	}

	// With no non-link egress at all, the app is link-only and default-drop as before.
	linkOnly := pastaApp()
	linkOnly.AppNameID = "client"
	linkOnly.NetworkMeta.NetworkLists = []schema.NetworkList{{AppName: "vpn"}}
	if !strings.Contains(NFTRuleset(linkOnly), "hook output priority 0; policy drop;") {
		t.Error("a link-only app must stay default-drop")
	}
}

// pasta cannot hold a second network - podman refuses outright - so an app with both gets a
// bridge instead. Its OWN bridge: apps sharing one can reach each other over L2, which
// would leave isolation resting on the nft rules, and an all-blacklist app runs
// default-accept.
func TestPodCreate_GatewayGetsItsOwnEgressBridge(t *testing.T) {
	steps := prepare(t, gatewayApp(), options.HostOptions{})

	if !slices.Contains(steps[0].Args, "zinc-egress-vpn") {
		t.Fatalf("the egress bridge should be created first, got %v", steps[0].Args)
	}
	if slices.Contains(steps[0].Args, "--internal") {
		t.Error("the egress bridge is the way out and must not be --internal")
	}

	var podArgs []string
	for _, step := range steps {
		if slices.Contains(step.Args, "pod") {
			podArgs = step.Args
		}
	}
	if !slices.Contains(podArgs, "zinc-egress-vpn:interface_name=zegress0") {
		t.Errorf("pod should attach its own egress bridge, got %v", podArgs)
	}
	if !slices.Contains(podArgs, "zinc-link-vpn:interface_name=zlink0,alias=vpn") {
		t.Errorf("pod should still attach its link, got %v", podArgs)
	}
	if slices.Contains(podArgs, "pasta") {
		t.Errorf("podman refuses pasta alongside a bridge, got %v", podArgs)
	}
}

// A link-only app must not gain a bridge to the outside just because the combination is now
// allowed - that would hand every existing tier-2 app egress it never asked for.
func TestPodCreate_LinkOnlyAppGetsNoEgressBridge(t *testing.T) {
	cfg := pastaApp()
	cfg.AppNameID = "db"
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{Ingress: true, Ports: []int{5432}}}

	for _, step := range prepare(t, cfg, options.HostOptions{}) {
		for _, arg := range step.Args {
			if strings.Contains(arg, "zinc-egress-") {
				t.Fatalf("a link-only app must stay on its private bridges alone, got %v", step.Args)
			}
		}
	}
}

// routedApp sends everything through a sibling; vpnApp agrees to carry it. Together they
// are the whole feature: the client has no other path to those destinations, so it cannot
// leak past the sibling, and if the sibling stops the traffic blackholes.
func routedApp() schema.AppConfig {
	cfg := pastaApp()
	cfg.AppNameID = "browser"
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{
		{AppName: "vpn", Via: true, IPv4CIDR: []string{"0.0.0.0/0"}},
	}
	return cfg
}

func vpnApp() schema.AppConfig {
	cfg := pastaApp()
	cfg.AppNameID = "vpn"
	cfg.NetworkMeta.NetworkLists = []schema.NetworkList{
		{Ingress: true, Forward: true},
		{IPv4CIDR: []string{"203.0.113.7/32"}, Ports: []int{51820}},
	}
	return cfg
}

// The gateway's address is never written into a config: podman assigns it and it changes
// when the gateway is recreated. The route step resolves it at launch through the network
// alias podman already gives every app on a link.
func TestVia_RouteResolvesTheGatewayAtLaunch(t *testing.T) {
	steps := prepare(t, routedApp(), options.HostOptions{})

	var script string
	for _, step := range steps {
		if strings.HasPrefix(step.Desc, "route through sibling") {
			script = step.Args[len(step.Args)-1]
		}
	}
	if script == "" {
		t.Fatal("a Via list should produce a route step")
	}
	for _, want := range []string{
		"getent hosts vpn", // resolved by alias, not from config
		`ip route replace 0.0.0.0/0 via "$gateway" dev zlink0`, // over the private link
		"set -e", // a route that fails must stop the launch
	} {
		if !strings.Contains(script, want) {
			t.Errorf("route script missing %q:\n%s", want, script)
		}
	}
}

// Order matters: resolving the gateway needs DNS, and the ruleset that follows closes the
// netns. Both still run before the app, so the app never sees an unlocked network.
func TestVia_RouteRunsBeforeTheRulesetAndBeforeTheApp(t *testing.T) {
	steps := prepare(t, routedApp(), options.HostOptions{})

	route, nft := -1, -1
	for index, step := range steps {
		switch {
		case strings.HasPrefix(step.Desc, "route through sibling"):
			route = index
		case strings.Contains(step.Desc, "lock netns"):
			nft = index
		}
	}
	if route < 0 || nft < 0 {
		t.Fatalf("expected both a route and an nft step, got %d steps", len(steps))
	}
	if route > nft {
		t.Error("the route step must run before the ruleset closes the netns, or DNS is already blocked")
	}
}

// A container cannot set ip_forward itself - /proc/sys is read-only in the namespace - so an
// app that agreed to route for its siblings would silently drop every packet it forwarded.
func TestForward_GatewayGetsForwardingAndNAT(t *testing.T) {
	steps := prepare(t, vpnApp(), options.HostOptions{})
	var podArgs []string
	for _, step := range steps {
		if slices.Contains(step.Args, "pod") {
			podArgs = step.Args
		}
	}
	if !slices.Contains(podArgs, "net.ipv4.ip_forward=1") {
		t.Errorf("a forwarding app needs ip_forward set at pod creation, got %v", podArgs)
	}

	got := NFTRuleset(vpnApp())
	for _, want := range []string{
		"hook forward priority 0; policy drop;",      // forwarded traffic is its own chain
		`iifname "zlink0" oifname "zegress0" accept`, // siblings out, nothing else
		"table ip nat {", // replies must come back
		`oifname "zegress0" masquerade`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("gateway ruleset missing %q:\n%s", want, got)
		}
	}
}

// An app that never agreed to route for anyone must not gain a forward chain, NAT, or
// forwarding sysctls. Forwarding for other apps makes this app a router, which is a
// privilege and must never be implied by another app naming it.
func TestForward_NotImpliedForAnOrdinaryApp(t *testing.T) {
	got := NFTRuleset(gatewayApp()) // has a link and egress, but no Forward
	for _, unwanted := range []string{"hook forward", "table ip nat"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("an app that did not opt in must not get %q:\n%s", unwanted, got)
		}
	}
	for _, step := range prepare(t, gatewayApp(), options.HostOptions{}) {
		if slices.Contains(step.Args, "net.ipv4.ip_forward=1") {
			t.Errorf("forwarding must be opt-in, got %v", step.Args)
		}
	}
}

// A routed app's resolver is not Zinc's to choose: podman writes resolv.conf and points it
// at the network's own DNS, which on an --internal bridge answers sibling names and forwards
// nothing. The query is redirected in the netns instead, to a resolver reached through the
// sibling - so it travels inside the tunnel and stops with it.
func TestDNS_RoutedAppsQueriesAreRedirectedThroughTheSibling(t *testing.T) {
	cfg := routedApp()
	cfg.NetworkMeta.DNSServers = []string{"1.1.1.1"}
	got := NFTRuleset(cfg)

	for _, want := range []string{
		"type nat hook output priority dstnat;", // before the filter hook, so it sees the new address
		"udp dport { 53, 853 } dnat to 1.1.1.1",
		"tcp dport { 53, 853 } dnat to 1.1.1.1",
		"ip daddr { 1.1.1.1 } udp dport { 53, 853 } accept", // and the filter then permits it
		"udp dport { 53, 853 } drop",                        // anything not redirected dies
	} {
		if !strings.Contains(got, want) {
			t.Errorf("routed ruleset missing %q:\n%s", want, got)
		}
	}
}

// Only a routed app. For an ordinary one the network's resolver works and is the only thing
// that knows its siblings' names, so redirecting would take that away for nothing.
func TestDNS_OrdinaryAppKeepsItsNetworkResolver(t *testing.T) {
	cfg := pastaApp()
	cfg.NetworkMeta.DNSServers = []string{"1.1.1.1"}
	got := NFTRuleset(cfg)

	if strings.Contains(got, "dnat to") {
		t.Errorf("an app that is not routed must keep its own resolver:\n%s", got)
	}
	// It is still held to the servers it declared.
	if !strings.Contains(got, "ip daddr { 1.1.1.1 } udp dport { 53, 853 } accept") {
		t.Errorf("declared resolvers should still be the only ones permitted:\n%s", got)
	}
}

// An app that declares nothing keeps exactly the ruleset it had before.
func TestDNS_NoServersNoRules(t *testing.T) {
	got := NFTRuleset(pastaApp())
	for _, unwanted := range []string{"dport { 53, 853 }", "dnat to"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("an app declaring no resolvers must be untouched, found %q:\n%s", unwanted, got)
		}
	}
}

// REGRESSION: a consumer-only app was left with no input base chain at all. In nftables a
// hook with no base chain is not filtered, so "no chain" means unfiltered inbound, not
// closed - and every app on that shared bridge could reach every port it listened on.
func TestNFTRuleset_ConsumerOnlyStillFiltersInbound(t *testing.T) {
	cfg := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer,
		AppNameID: "client", ImageMeta: schema.ImageMeta{Image: "localhost/c:local"},
		NetworkMeta: schema.NetworkMeta{NetworkLists: []schema.NetworkList{{AppName: "db"}}},
	}
	ruleset := NFTRuleset(cfg)
	if !strings.Contains(ruleset, "chain input") {
		t.Fatalf("a linked app must still have an input chain, or its inbound is unfiltered:\n%s", ruleset)
	}
	if !strings.Contains(ruleset, "type filter hook input priority 0; policy drop;") {
		t.Errorf("a consumer publishes nothing, so its input chain must default-drop:\n%s", ruleset)
	}
}

// The declared resolvers are handed to every filtered app. The ruleset restricts DNS to them
// for all of them, so an app restricted but never given them has no DNS at all.
func TestPodCreateArgs_DNSReachesAnUnlinkedApp(t *testing.T) {
	cfg := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer,
		AppNameID: "plain", ImageMeta: schema.ImageMeta{Image: "localhost/p:local"},
		NetworkMeta: schema.NetworkMeta{
			DNSServers:   []string{"10.0.0.53"},
			NetworkLists: []schema.NetworkList{{IPv4CIDR: []string{"0.0.0.0/0"}, Ports: []int{443}}},
		},
	}
	args := strings.Join(podCreateArgs(cfg, "plain-pod"), " ")
	if !strings.Contains(args, "--dns 10.0.0.53") {
		t.Fatalf("an unlinked app must be handed its declared resolvers too: %s", args)
	}
}

// The resolver a routed app's DNS is redirected to has to travel through the sibling like
// everything else. Without a route for it, every query leaves by the app's own egress in the
// clear while the docs say it goes through the tunnel.
func TestRouteCommands_RedirectedResolverIsRoutedThroughTheSibling(t *testing.T) {
	cfg := routedApp()
	cfg.NetworkMeta.DNSServers = []string{"10.0.0.53"}
	var script string
	for _, step := range routeCommands(cfg, "zinc/netfilter:local") {
		script += strings.Join(step.Args, " ")
	}
	if !strings.Contains(script, "ip route replace 10.0.0.53/32 via") {
		t.Fatalf("the redirected resolver must get a route through the sibling:\n%s", script)
	}
}

// The rendered ruleset must be identical across runs; map iteration order used to leak into
// it whenever both DNS families were declared.
func TestNFTRuleset_Deterministic(t *testing.T) {
	cfg := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer,
		AppNameID: "app", ImageMeta: schema.ImageMeta{Image: "localhost/a:local"},
		NetworkMeta: schema.NetworkMeta{
			DNSServers:   []string{"1.1.1.1", "2606:4700:4700::1111"},
			NetworkLists: []schema.NetworkList{{IPv4CIDR: []string{"0.0.0.0/0"}, Ports: []int{443}}},
		},
	}
	first := NFTRuleset(cfg)
	for run := 0; run < 200; run++ {
		if got := NFTRuleset(cfg); got != first {
			t.Fatalf("ruleset differs between renders:\n%s\n---\n%s", first, got)
		}
	}
}

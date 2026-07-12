// Package netenforce holds the NetEnforcer adapter — the swappable egress mechanism.
// It implements ports.NetEnforcer: how the app container attaches to the network
// (RunFlags), what must happen to establish and LOCK the netns before the app starts
// (Prepare), and how to tear it down (Teardown).
//
// One mechanism ships today: an app's NetworkLists are enforced as an nftables
// allow/deny ruleset on the app's own pasta netns (a pod). An app with no
// NetworkLists gets --network none. A future mechanism — eBPF egress, a proxy
// sidecar, an external traffic controller — is one more file here implementing the
// same interface; nothing in app or the podman runtime changes (docs §5.3, §13).
//
// Scope (this build): only self-scoped lists (Host=false, empty AppName, no gateway)
// are enforceable. Host-scoped, sibling (AppName), and gateway (multi-homing) lists
// are schema-legal but deferred; the app layer's checkNetwork rejects a config using
// them before this adapter runs, so every list reaching here is self-scoped.
package netenforce

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

// Compile-time check that the enforcer satisfies ports.NetEnforcer.
var _ ports.NetEnforcer = Enforcer{}

// DefaultNetfilterImage is the locally built helper that carries nft. It runs once
// per filtered launch to lock the pod's netns before the app starts. Build it with
// `make netfilter-image`. The nft step runs it with --pull=never (see nftApplyArgs):
// the privileged helper is always the locally vetted build, never pulled from a
// registry, and a missing image fails fast with a clear error. The tag must match the
// netfilter image build (creator side).
const DefaultNetfilterImage = "zinc/netfilter:local"

// Enforcer drives an app's NetworkLists onto the network. It satisfies
// ports.NetEnforcer and is stateless.
type Enforcer struct{}

// PodName is the pod that owns a filtered app's netns.
func PodName(app string) string { return app + "-pod" }

// filtered reports whether cfg needs a pasta pod + nft: any NetworkList present. An
// app with none gets --network none. checkNetwork (app layer) has already rejected
// host/sibling/gateway lists, so any list here is self-scoped and enforceable.
func filtered(cfg schema.AppConfig) bool {
	return len(cfg.NetworkMeta.NetworkLists) > 0
}

// RunFlags attaches the app container to the network. Filtered: join the pasta pod
// (its infra container owns networking and the nft ruleset is already in place from
// Prepare, so the app only joins the locked netns — no per-app --network, no net
// caps). Unfiltered: --network none.
func (Enforcer) RunFlags(cfg schema.AppConfig) []string {
	if filtered(cfg) {
		return []string{"--pod", PodName(cfg.AppNameID)}
	}
	return []string{"--network", "none"}
}

// Prepare returns the steps that guarantee no unfiltered-egress window (§5.3): create
// the pod (pasta netns), then lock the netns with nft *before any app starts*. The
// app run itself is appended by the caller (app layer) using RunFlags. An unfiltered
// app has nothing to prepare.
func (Enforcer) Prepare(cfg schema.AppConfig, opt options.HostOptions) []ports.Command {
	if !filtered(cfg) {
		return nil
	}
	pod := PodName(cfg.AppNameID)
	image := opt.NetfilterImage
	if image == "" {
		image = DefaultNetfilterImage
	}
	return []ports.Command{
		{Args: podCreateArgs(cfg, pod), Desc: "create pod (pasta netns)"},
		{Args: nftApplyArgs(pod, image), Stdin: NFTRuleset(cfg), Desc: "lock netns with nft (before app)"},
	}
}

// Teardown removes the pod (owns the filtered netns — app and firewall go in one
// step, no stale rule-less netns left behind), or just stops the container for an
// unfiltered app.
func (Enforcer) Teardown(cfg schema.AppConfig) []string {
	if filtered(cfg) {
		return []string{"pod", "rm", "-f", PodName(cfg.AppNameID)}
	}
	return []string{"stop", cfg.AppNameID}
}

// NFTRuleset renders the nftables ruleset enforcing an app's NetworkLists (§5.3).
// Pure function over the validated config; loaded into the pod's own netns by the
// netfilter init step, before the app container starts — so the app never sees an
// open network.
//
// Chain policy follows the lists' orientation: a whitelist ("only these") means
// default-drop, so the app that lists an allowlist is fail-closed; an all-blacklist
// app ("all but these") means default-accept — allow-all-except. Any whitelist
// present flips the whole app to restrictive default-drop (see allBlacklist), so a
// mixed app never silently opens.
//
// Loopback and established/related return traffic are always accepted. Then each
// NetworkList contributes rules in priority order (first entry first), first match
// wins (nft evaluates top-down; accept/drop are terminal): a whitelist list accepts
// its CIDRs/ports, a blacklist list drops them. Blocking DNS is just a blacklist list
// for ports 53/853 (validate rejects a port rule with no CIDR, so it cannot silently
// no-op), ordered ahead of any broad allow so it wins.
func NFTRuleset(cfg schema.AppConfig) string {
	policy := "drop"
	if allBlacklist(cfg.NetworkMeta.NetworkLists) {
		policy = "accept" // allow-all-except: only the listed CIDRs/ports are dropped
	}
	var bld strings.Builder
	bld.WriteString("table inet zinc {\n")
	bld.WriteString("\tchain output {\n")
	fmt.Fprintf(&bld, "\t\ttype filter hook output priority 0; policy %s;\n", policy)
	bld.WriteString("\t\toif \"lo\" accept\n")
	bld.WriteString("\t\tct state established,related accept\n")
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		verdict := "accept"
		if netList.Blacklist {
			verdict = "drop"
		}
		writeRules(&bld, "ip", netList.IPv4CIDR, netList.Ports, verdict)
		writeRules(&bld, "ip6", netList.IPv6CIDR, netList.Ports, verdict)
	}
	bld.WriteString("\t}\n")
	bld.WriteString("}\n")
	return bld.String()
}

// allBlacklist reports whether every list is a blacklist, which makes the chain
// default accept (allow-all-except). A single whitelist present returns false, so the
// app is restrictive (default-drop) and the blacklist lists become high-priority
// deny carve-outs above the whitelist's accepts. Callers only invoke NFTRuleset for a
// filtered app, so the slice is non-empty here.
func allBlacklist(lists []schema.NetworkList) bool {
	for _, netList := range lists {
		if !netList.Blacklist {
			return false
		}
	}
	return true
}

// writeRules emits the verdict rules for one address family. No CIDRs → nothing.
// Ports listed → only those ports (tcp+udp); otherwise all ports to the listed CIDRs.
func writeRules(bld *strings.Builder, family string, cidrs []string, ports []int, verdict string) {
	if len(cidrs) == 0 {
		return
	}
	daddr := family + " daddr { " + strings.Join(cidrs, ", ") + " }"
	if len(ports) == 0 {
		fmt.Fprintf(bld, "\t\t%s %s\n", daddr, verdict)
		return
	}
	portsList := portList(ports)
	fmt.Fprintf(bld, "\t\t%s tcp dport { %s } %s\n", daddr, portsList, verdict)
	fmt.Fprintf(bld, "\t\t%s udp dport { %s } %s\n", daddr, portsList, verdict)
}

func portList(ports []int) string {
	strs := make([]string, len(ports))
	for idx, port := range ports {
		strs[idx] = strconv.Itoa(port)
	}
	return strings.Join(strs, ", ")
}

// podCreateArgs builds `podman pod create` for a filtered app's netns. When a list
// names a host interface, pasta copies its addressing (the first such interface wins).
func podCreateArgs(cfg schema.AppConfig, pod string) []string {
	netspec := "pasta"
	if iface := firstInterface(cfg); iface != "" {
		netspec = "pasta:--interface," + iface
	}
	return []string{"pod", "create", "--name", pod, "--network", netspec}
}

// firstInterface returns the first non-blank Interface across the app's NetworkLists.
func firstInterface(cfg schema.AppConfig) string {
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if iface := strings.TrimSpace(netList.Interface); iface != "" {
			return iface
		}
	}
	return ""
}

// nftApplyArgs builds the one-shot init `podman run` that loads the ruleset into the
// pod's netns. It carries only NET_ADMIN — namespaced to the pod's userns, so
// harmless on the host (§5.3) — reads the ruleset from stdin, and exits.
func nftApplyArgs(pod, image string) []string {
	return []string{
		"run", "--pod", pod, "--rm", "-i", "--pull", "never",
		"--security-opt", "no-new-privileges", "--cap-drop", "all", "--cap-add", "NET_ADMIN",
		image, "nft", "-f", "-",
	}
}

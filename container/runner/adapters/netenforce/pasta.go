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
// Scope (this build): self-scoped egress lists (Host=false, empty AppName) and tier-3
// LAN publishing (Ingress && Host — an nft input chain plus pod `-p` forwards). Deferred
// and rejected by the app layer's checkNetwork before this adapter runs: self-scoped
// ingress (tier-2 sibling reach), host-scoped egress, sibling (AppName), and gateway
// (multi-homing) lists.
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
// A list's direction picks its chain: an egress list (Ingress=false) becomes an output
// rule (where the app may connect to — daddr), an ingress list (Ingress=true) becomes
// an input rule (who may connect in to the app's published ports — saddr). Egress lists
// build the output chain, ingress lists the input chain; each is sized independently.
//
// Per-direction chain policy follows that direction's lists: a whitelist ("only these")
// means default-drop (fail-closed); an all-blacklist direction ("all but these") means
// default-accept. A single whitelist present flips the direction to restrictive
// default-drop (see chainPolicy/allBlacklist), so it never silently opens. A direction
// with no lists is default-drop — a pure publisher gets no egress, an egress-only app
// gets no input chain at all.
//
// Loopback and established/related traffic are always accepted (a server's reply rides
// the established output rule). Then each list contributes rules in priority order,
// first match wins. Blocking DNS is just an egress blacklist for ports 53/853 (validate
// rejects a port rule with no CIDR, so it cannot silently no-op), ordered ahead of any
// broad allow so it wins.
func NFTRuleset(cfg schema.AppConfig) string {
	var egress, ingress []schema.NetworkList
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if netList.Ingress {
			ingress = append(ingress, netList)
		} else {
			egress = append(egress, netList)
		}
	}

	var bld strings.Builder
	bld.WriteString("table inet zinc {\n")

	// output (egress): where the app may connect out to.
	bld.WriteString("\tchain output {\n")
	fmt.Fprintf(&bld, "\t\ttype filter hook output priority 0; policy %s;\n", chainPolicy(egress))
	bld.WriteString("\t\toif \"lo\" accept\n")
	bld.WriteString("\t\tct state established,related accept\n")
	for _, netList := range egress {
		verdict := verdictFor(netList)
		writeRules(&bld, "ip", netList.IPv4CIDR, netList.Ports, verdict)
		writeRules(&bld, "ip6", netList.IPv6CIDR, netList.Ports, verdict)
	}
	bld.WriteString("\t}\n")

	// input (ingress): who may reach the app's published ports. Emitted only when the
	// app publishes; without it there is no input base chain, so ingress stays closed.
	if len(ingress) > 0 {
		bld.WriteString("\tchain input {\n")
		fmt.Fprintf(&bld, "\t\ttype filter hook input priority 0; policy %s;\n", chainPolicy(ingress))
		bld.WriteString("\t\tiif \"lo\" accept\n")
		bld.WriteString("\t\tct state established,related accept\n")
		for _, netList := range ingress {
			writeIngressRules(&bld, netList, verdictFor(netList))
		}
		bld.WriteString("\t}\n")
	}

	bld.WriteString("}\n")
	return bld.String()
}

// verdictFor is the terminal verdict a list contributes: a whitelist accepts its
// matches, a blacklist drops them.
func verdictFor(netList schema.NetworkList) string {
	if netList.Blacklist {
		return "drop"
	}
	return "accept"
}

// chainPolicy is the default policy for one direction's lists: default-accept only when
// there is at least one list and every one is a blacklist (allow-all-except); otherwise
// default-drop. An empty direction is default-drop (closed).
func chainPolicy(lists []schema.NetworkList) string {
	if len(lists) > 0 && allBlacklist(lists) {
		return "accept"
	}
	return "drop"
}

// allBlacklist reports whether every list is a blacklist. A single whitelist present
// returns false, so the direction is restrictive (default-drop) and the blacklist lists
// become high-priority deny carve-outs above the whitelist's accepts.
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

// writeIngressRules emits input-chain rules for one ingress list: match the app's own
// published Ports, restricted to the source CIDRs (saddr). Unlike egress, an empty CIDR
// is legal and means "any source" (validate exempts ingress from the ports-need-CIDR
// rule), so a list with ports but no CIDR opens those ports to anyone the pod forwards.
// v4 and v6 source sets are emitted separately so a v4 CIDR never gates v6 traffic.
func writeIngressRules(bld *strings.Builder, netList schema.NetworkList, verdict string) {
	ports := portList(netList.Ports)
	emit := func(saddr string) {
		switch {
		case ports == "" && saddr == "":
			return // nothing to match
		case ports == "":
			fmt.Fprintf(bld, "\t\t%s %s\n", saddr, verdict)
		case saddr == "":
			fmt.Fprintf(bld, "\t\ttcp dport { %s } %s\n", ports, verdict)
			fmt.Fprintf(bld, "\t\tudp dport { %s } %s\n", ports, verdict)
		default:
			fmt.Fprintf(bld, "\t\t%s tcp dport { %s } %s\n", saddr, ports, verdict)
			fmt.Fprintf(bld, "\t\t%s udp dport { %s } %s\n", saddr, ports, verdict)
		}
	}
	hasV4, hasV6 := len(netList.IPv4CIDR) > 0, len(netList.IPv6CIDR) > 0
	if !hasV4 && !hasV6 {
		emit("") // any source
		return
	}
	if hasV4 {
		emit("ip saddr { " + strings.Join(netList.IPv4CIDR, ", ") + " }")
	}
	if hasV6 {
		emit("ip6 saddr { " + strings.Join(netList.IPv6CIDR, ", ") + " }")
	}
}

func portList(ports []int) string {
	strs := make([]string, len(ports))
	for idx, port := range ports {
		strs[idx] = strconv.Itoa(port)
	}
	return strings.Join(strs, ", ")
}

// podCreateArgs builds `podman pod create` for a filtered app's netns. When a list
// names a host interface, pasta copies its addressing (the first such interface wins),
// which also scopes tier-3 publishing to that interface. Tier-3 (LAN) ingress lists add
// their ports as `-p` forwards here (pod ports live on the pod, not the container).
func podCreateArgs(cfg schema.AppConfig, pod string) []string {
	netspec := "pasta"
	if iface := firstInterface(cfg); iface != "" {
		netspec = "pasta:--interface," + iface
	}
	args := []string{"pod", "create", "--name", pod, "--network", netspec}
	return append(args, publishArgs(cfg)...)
}

// publishArgs maps tier-3 (LAN) ingress lists — Ingress && Host — onto pod `-p` port
// forwards so the LAN can reach the app's published ports; the nft input chain then
// restricts who (source CIDR) actually gets through, and pasta binds the pod's interface
// (firstInterface). Each port is forwarded for both tcp and udp to match the input
// chain; there is no host-port remap (published port == container port). Self-scoped
// ingress (tier 2) publishes nothing to the host and never reaches here — checkNetwork
// rejects it in this build.
func publishArgs(cfg schema.AppConfig) []string {
	var args []string
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if !netList.Ingress || !netList.Host {
			continue
		}
		for _, port := range netList.Ports {
			mapping := strconv.Itoa(port) + ":" + strconv.Itoa(port)
			args = append(args, "-p", mapping+"/tcp", "-p", mapping+"/udp")
		}
	}
	return args
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

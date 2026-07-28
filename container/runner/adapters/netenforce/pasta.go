// Package netenforce holds the NetEnforcer adapter - the swappable egress mechanism.
// It implements ports.NetEnforcer: how the app container attaches to the network
// (RunFlags), what must happen to establish and LOCK the netns before the app starts
// (Prepare), and how to tear it down (Teardown).
//
// One mechanism ships today: an app's NetworkLists are enforced as an nftables
// allow/deny ruleset on the app's own pasta netns (a pod). An app with no
// NetworkLists gets --network none. A future mechanism - eBPF egress, a proxy
// sidecar, an external traffic controller - is one more file here implementing the
// same interface; nothing in app or the podman runtime changes (docs section 5.3, section 13).
//
// Scope (this build): self-scoped egress lists (Host=false, empty AppName), tier-3 LAN
// publishing (Ingress && Host - an nft input chain plus pod `-p` forwards), and tier-2
// sibling links (a private --internal bridge per producer, interface-gated per-port nft;
// a producer's self-scoped ingress + a consumer's egress naming its AppName). checkNetwork
// forbids mixing tier-2 with other networking, and still rejects host-scoped egress and
// gateway (multi-homing) lists before this adapter runs.
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
// netfilter image build (runner side).
const DefaultNetfilterImage = "zinc/netfilter:local"

// Enforcer drives an app's NetworkLists onto the network. It satisfies ports.NetEnforcer.
//
// Lookup resolves the Domains an egress list allows by name; a zero Enforcer uses the host
// resolver, which is what production wants and what every existing caller gets. It is a
// field so the resolution can be driven without a network in tests.
type Enforcer struct{ Lookup LookupFunc }

// PodName is the pod that owns a filtered app's netns.
func PodName(app string) string { return app + "-pod" }

// LinkNetwork is the private, --internal bridge that connects a producer to the siblings
// that consume it (tier 2). A producer owns zinc-link-<self>; a consumer attaches to the
// producer's. The name is deterministic so both sides agree without coordination.
func LinkNetwork(producer string) string { return "zinc-link-" + producer }

// EgressNetwork is an app's own bridge to the outside, used only when it has both sibling
// links and networking that must leave them. One per app rather than the shared default
// bridge, so two apps are never on the same L2 segment.
func EgressNetwork(app string) string { return "zinc-egress-" + app }

// egressIface is the fixed in-container name of that bridge, so the ruleset and the pod
// attach agree on it the way they do for zlink0..N.
const egressIface = "zegress0"

// linkEntry is one bridge a tier-2 app attaches to, paired with the fixed in-container
// interface name (zlink0, zlink1, ...) used both for `--network name:interface_name=` and
// for the nft rules that gate that interface (validated: podman names it exactly that).
type linkEntry struct {
	network string
	iface   string
}

// links returns the ordered bridges a tier-2 app attaches to: one per self-scoped
// ingress list (its own link, as a producer) and one per sibling it consumes (an egress
// list naming an AppName), de-duplicated. Empty for a non-tier-2 app. The slice index
// fixes each link's interface name, so the ruleset and the pod attach agree.
func links(cfg schema.AppConfig) []linkEntry {
	var out []linkEntry
	seen := map[string]bool{}
	add := func(network string) {
		if seen[network] {
			return
		}
		seen[network] = true
		out = append(out, linkEntry{network: network, iface: "zlink" + strconv.Itoa(len(out))})
	}
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		appName := strings.TrimSpace(netList.AppName)
		switch {
		case netList.Ingress && !netList.Host && appName == "":
			add(LinkNetwork(cfg.AppNameID)) // producer: own link
		case !netList.Ingress && !netList.Host && appName != "":
			add(LinkNetwork(appName)) // consumer: the producer's link
		}
	}
	return out
}

// ownLinkIface is the interface of an app's own producer link (zinc-link-<self>), or ""
// when the app is not a producer. The app's published ports are accepted on it.
func ownLinkIface(cfg schema.AppConfig) string {
	own := LinkNetwork(cfg.AppNameID)
	for _, entry := range links(cfg) {
		if entry.network == own {
			return entry.iface
		}
	}
	return ""
}

// filtered reports whether cfg needs a netns + nft: any NetworkList present. An app with
// none gets --network none. checkNetwork (app layer) has already rejected the scopes this
// build can't enforce, so every list reaching here is one the enforcer handles.
func filtered(cfg schema.AppConfig) bool {
	return len(cfg.NetworkMeta.NetworkLists) > 0
}

// RunFlags attaches the app container to the network. Filtered: join the pasta pod
// (its infra container owns networking and the nft ruleset is already in place from
// Prepare, so the app only joins the locked netns - no per-app --network, no net
// caps). Unfiltered: --network none.
func (Enforcer) RunFlags(cfg schema.AppConfig) []string {
	if filtered(cfg) {
		return []string{"--pod", PodName(cfg.AppNameID)}
	}
	return []string{"--network", "none"}
}

// Prepare returns the steps that guarantee no unfiltered window (section 5.3): ensure any tier-2
// link bridges exist, create the pod (its netns), then lock the netns with nft *before
// any app starts*. The app run itself is appended by the caller (app layer) using
// RunFlags. An unfiltered app has nothing to prepare. Link networks are created
// idempotently (--ignore) and left in place on teardown - a sibling may still use one.
func (enf Enforcer) Prepare(cfg schema.AppConfig, opt options.HostOptions) ([]ports.Command, error) {
	if !filtered(cfg) {
		return nil, nil
	}
	// Names become addresses before anything is rendered, so the ruleset is only ever built
	// from addresses and the renderer stays pure. It happens here rather than inside the
	// renderer because it is the one part of building a ruleset that can fail, and a launch
	// whose allowlist could not be resolved must not proceed with a shorter one.
	cfg, err := resolveDomains(cfg, enf.Lookup)
	if err != nil {
		return nil, err
	}
	pod := PodName(cfg.AppNameID)
	image := opt.NetfilterImage
	if image == "" {
		image = DefaultNetfilterImage
	}
	var steps []ports.Command
	if entries := links(cfg); len(entries) > 0 && needsOwnEgress(cfg) {
		// Not --internal: this is the one bridge such an app reaches the outside through.
		steps = append(steps, ports.Command{
			Args: []string{"network", "create", "--ignore", EgressNetwork(cfg.AppNameID)},
			Desc: "ensure egress network " + EgressNetwork(cfg.AppNameID),
		})
	}
	for _, entry := range links(cfg) {
		steps = append(steps, ports.Command{
			Args: []string{"network", "create", "--ignore", "--internal", entry.network},
			Desc: "ensure link network " + entry.network,
		})
	}
	steps = append(steps, ports.Command{Args: podCreateArgs(cfg, pod), Desc: "create pod (netns)"})
	// Routes first, rules second: resolving the gateway needs DNS, and the ruleset that
	// follows closes the netns. Both are done before the app starts, so the app still never
	// sees an unlocked network.
	steps = append(steps, routeCommands(cfg, image)...)
	return append(steps,
		ports.Command{Args: nftApplyArgs(pod, image), Stdin: NFTRuleset(cfg), Desc: "lock netns with nft (before app)"},
	), nil
}

// Teardown removes the pod (owns the filtered netns - app and firewall go in one
// step, no stale rule-less netns left behind), or just stops the container for an
// unfiltered app.
func (Enforcer) Teardown(cfg schema.AppConfig) []string {
	if filtered(cfg) {
		return []string{"pod", "rm", "-f", PodName(cfg.AppNameID)}
	}
	return []string{"stop", cfg.AppNameID}
}

// viaLists returns the egress lists that route through a sibling, paired with the link
// interface their traffic leaves by. The interface comes from links(), so the route and the
// ruleset always agree on which bridge is which.
func viaLists(cfg schema.AppConfig) []viaRoute {
	byNetwork := map[string]string{}
	for _, entry := range links(cfg) {
		byNetwork[entry.network] = entry.iface
	}
	var out []viaRoute
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if !netList.Via {
			continue
		}
		gateway := strings.TrimSpace(netList.AppName)
		out = append(out, viaRoute{
			gateway: gateway,
			iface:   byNetwork[LinkNetwork(gateway)],
			cidrs:   append(append([]string{}, netList.IPv4CIDR...), netList.IPv6CIDR...),
		})
	}
	return out
}

// viaRoute is one "send these destinations through that sibling" instruction.
type viaRoute struct {
	gateway string
	iface   string
	cidrs   []string
}

// forwards reports whether this app has agreed to route for the siblings on its link.
func forwards(cfg schema.AppConfig) bool {
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if netList.Forward {
			return true
		}
	}
	return false
}

// routeCommands installs the sibling routes inside the app's netns, before the app starts.
//
// The gateway's address is resolved at this moment rather than written into the config,
// because podman assigns it and it changes when the gateway is recreated. It resolves by the
// network alias podman already gives every app on a link (its AppNameID), using the link's
// own DNS - so a config never carries an address, and a gateway that restarts on a new one
// is picked up the next time a client starts.
//
// The step runs in the same helper as the ruleset and before it, so DNS is still reachable:
// afterwards the netns is default-drop. Both run before the app, so the app still never sees
// an unlocked network.
func routeCommands(cfg schema.AppConfig, image string) []ports.Command {
	var steps []ports.Command
	for _, route := range viaLists(cfg) {
		if len(route.cidrs) == 0 || route.iface == "" {
			continue
		}
		var script strings.Builder
		// Fail on the first error: a route that did not install would leave the app sending
		// those destinations out its own egress instead - the leak this feature exists to
		// prevent - so the launch must stop rather than continue quietly.
		script.WriteString("set -e\n")
		fmt.Fprintf(&script, "gateway=$(getent hosts %s | awk '{print $1; exit}')\n", route.gateway)
		fmt.Fprintf(&script, "test -n \"$gateway\" || { echo \"cannot resolve sibling %s on its link\" >&2; exit 1; }\n", route.gateway)
		for _, cidr := range route.cidrs {
			// replace, not add: a re-run of a resumed launch must not fail on an existing
			// route, and the default route already exists on a bridge-attached pod.
			fmt.Fprintf(&script, "ip route replace %s via \"$gateway\" dev %s\n", cidr, route.iface)
		}
		steps = append(steps, ports.Command{
			Args: []string{
				"run", "--pod", PodName(cfg.AppNameID), "--rm", "--pull", "never",
				"--security-opt", "no-new-privileges", "--cap-drop", "all", "--cap-add", "NET_ADMIN",
				image, "sh", "-c", script.String(),
			},
			Desc: "route through sibling " + route.gateway,
		})
	}
	return steps
}

// isLinkList reports whether a list is a tier-2 sibling link: a producer's self-scoped
// ingress, or a consumer's egress naming a sibling. Mirrors app.isLinkList; the two live
// apart because the app layer decides what is allowed and this layer decides what is
// rendered.
func isLinkList(netList schema.NetworkList) bool {
	appName := strings.TrimSpace(netList.AppName)
	producer := netList.Ingress && !netList.Host && appName == ""
	consumer := !netList.Ingress && !netList.Host && appName != ""
	return producer || consumer
}

// NFTRuleset renders the nftables ruleset locked into an app's netns before it starts
// (section 5.3). Pure over the validated config.
//
// One app can now be gated both ways at once, which is what routing through a sibling
// needs: the private zlink* bridges are accepted by INTERFACE, and everything else is
// gated by ADDRESS and port. Before, an app was one or the other and mixing them was
// rejected at launch, because whichever ruleset ran would have ignored the other kind of
// list entirely - the address rules of a linked app simply vanished.
//
// Chain policy comes from the NON-link lists alone. A link list is structurally a
// whitelist (validation refuses a blacklist one), so folding it into the policy decision
// would flip an app that pairs a link with an all-blacklist egress from default-accept to
// default-drop and silently deny everything the blacklist meant to leave open. With no
// non-link lists in a direction the policy stays drop, which is what a link-only app has
// always had.
func NFTRuleset(cfg schema.AppConfig) string {
	var egress, ingress []schema.NetworkList
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if isLinkList(netList) {
			continue // gated by interface below, not by address
		}
		if netList.Ingress {
			ingress = append(ingress, netList)
		} else {
			egress = append(egress, netList)
		}
	}
	linkEntries := links(cfg)

	var bld strings.Builder
	bld.WriteString("table inet zinc {\n")

	// output (egress): where the app may connect out to.
	bld.WriteString("\tchain output {\n")
	fmt.Fprintf(&bld, "\t\ttype filter hook output priority 0; policy %s;\n", chainPolicy(egress))
	bld.WriteString("\t\toif \"lo\" accept\n")
	bld.WriteString("\t\tct state established,related accept\n")
	// DNS first, so it is decided before the broad accepts below can let a query past.
	// The declared resolvers are reachable and every other one is not, which is what stops
	// an app that carries a hardcoded resolver from stepping around them - it matters once
	// an app has direct egress of its own alongside a routed link.
	writeDNSRules(&bld, cfg.NetworkMeta.DNSServers)
	// The link bridges are accepted whole: they are private and --internal, so what rides
	// them can only reach the siblings attached to them.
	for _, entry := range linkEntries {
		fmt.Fprintf(&bld, "\t\toifname %q accept\n", entry.iface)
	}
	for _, netList := range egress {
		verdict := verdictFor(netList)
		writeRules(&bld, "ip", netList.IPv4CIDR, netList.Ports, verdict)
		writeRules(&bld, "ip6", netList.IPv6CIDR, netList.Ports, verdict)
	}
	bld.WriteString("\t}\n")

	// input (ingress): who may reach the app's published ports. Emitted when the app
	// publishes to the LAN or serves siblings on its own link; without either there is no
	// input base chain at all, so ingress stays closed.
	own := ownLinkIface(cfg)
	if len(ingress) > 0 || own != "" {
		bld.WriteString("\tchain input {\n")
		fmt.Fprintf(&bld, "\t\ttype filter hook input priority 0; policy %s;\n", chainPolicy(ingress))
		bld.WriteString("\t\tiif \"lo\" accept\n")
		bld.WriteString("\t\tct state established,related accept\n")
		if own != "" {
			for _, netList := range cfg.NetworkMeta.NetworkLists {
				if isLinkList(netList) && netList.Ingress && len(netList.Ports) > 0 {
					fmt.Fprintf(&bld, "\t\tiifname %q tcp dport { %s } accept\n", own, portList(netList.Ports))
					fmt.Fprintf(&bld, "\t\tiifname %q udp dport { %s } accept\n", own, portList(netList.Ports))
				}
			}
		}
		for _, netList := range ingress {
			writeIngressRules(&bld, netList, verdictFor(netList))
		}
		bld.WriteString("\t}\n")
	}

	// A forwarding app needs two more things, and neither is optional: a filter rule for
	// traffic it routes on behalf of siblings (the forward hook is a separate chain from
	// output, so the egress rules above never see it), and NAT out of its own bridge, or
	// replies would be addressed to a private link address the outside cannot route back to.
	if forwards(cfg) {
		bld.WriteString("\tchain forward {\n")
		bld.WriteString("\t\ttype filter hook forward priority 0; policy drop;\n")
		bld.WriteString("\t\tct state established,related accept\n")
		if own := ownLinkIface(cfg); own != "" {
			fmt.Fprintf(&bld, "\t\tiifname %q oifname %q accept\n", own, egressIface)
		}
		bld.WriteString("\t}\n")
	}
	bld.WriteString("}\n")

	if forwards(cfg) || dnsRedirect(cfg) != "" {
		bld.WriteString("table ip nat {\n")
		if forwards(cfg) {
			bld.WriteString("\tchain postrouting {\n")
			bld.WriteString("\t\ttype nat hook postrouting priority srcnat; policy accept;\n")
			fmt.Fprintf(&bld, "\t\toifname %q masquerade\n", egressIface)
			bld.WriteString("\t}\n")
		}
		// A routed app's resolver is not ours to choose: podman writes resolv.conf and
		// points it at the network's own DNS, which on an --internal bridge answers sibling
		// names and forwards nothing (measured: an external name returns NXDOMAIN). Rather
		// than fight over the file, the query is redirected here, to a resolver the app
		// reaches through its sibling - so it travels inside the tunnel and stops with it.
		// dstnat runs before the filter hook, so the rules above then see the new address.
		if server := dnsRedirect(cfg); server != "" {
			bld.WriteString("\tchain output {\n")
			bld.WriteString("\t\ttype nat hook output priority dstnat; policy accept;\n")
			for _, proto := range []string{"udp", "tcp"} {
				fmt.Fprintf(&bld, "\t\t%s dport { 53, 853 } dnat to %s\n", proto, server)
			}
			bld.WriteString("\t}\n")
		}
		bld.WriteString("}\n")
	}
	return bld.String()
}

// dnsRedirect returns the resolver a routed app's DNS is rewritten to, or "" when the app
// is not routed. Only a routed app: for an ordinary one the network's resolver works and is
// the only thing that knows its siblings' names, so redirecting would take that away for
// nothing. A routed app has already lost it - that resolver cannot answer anything external
// from an internal bridge - which is why this is a repair rather than a restriction.
//
// The first declared server: validation requires a routed app to name one, and one address
// is what a dnat rule takes.
func dnsRedirect(cfg schema.AppConfig) string {
	if len(cfg.NetworkMeta.DNSServers) == 0 {
		return ""
	}
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if netList.Via {
			return strings.TrimSpace(cfg.NetworkMeta.DNSServers[0])
		}
	}
	return ""
}

// writeDNSRules permits DNS to the declared resolvers and drops it everywhere else. The
// drop is the point: without it, naming a resolver would be a suggestion rather than a
// restriction, and an app is free to ignore what its /etc/resolv.conf says.
//
// Emitted only when the app declares resolvers - an app that names none keeps whatever DNS
// its network gives it, exactly as before.
func writeDNSRules(bld *strings.Builder, servers []string) {
	if len(servers) == 0 {
		return
	}
	var v4, v6 []string
	for _, server := range servers {
		address := strings.TrimSpace(server)
		if strings.Contains(address, ":") {
			v6 = append(v6, address)
		} else {
			v4 = append(v4, address)
		}
	}
	for family, addresses := range map[string][]string{"ip": v4, "ip6": v6} {
		if len(addresses) == 0 {
			continue
		}
		for _, proto := range []string{"udp", "tcp"} {
			fmt.Fprintf(bld, "\t\t%s daddr { %s } %s dport { 53, 853 } accept\n",
				family, strings.Join(addresses, ", "), proto)
		}
	}
	for _, proto := range []string{"udp", "tcp"} {
		fmt.Fprintf(bld, "\t\t%s dport { 53, 853 } drop\n", proto)
	}
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

// podCreateArgs builds `podman pod create` for a filtered app's netns. A tier-2 app
// attaches to its private link bridge(s), each pinned to a fixed interface name the nft
// rules match (no pasta, no host publish - checkNetwork forbids mixing). Otherwise the
// pod is a pasta netns: a list naming a host interface makes pasta copy its addressing
// (first wins), which also scopes tier-3 publishing, and tier-3 (LAN) ingress lists add
// their ports as `-p` forwards here (pod ports live on the pod, not the container).
func podCreateArgs(cfg schema.AppConfig, pod string) []string {
	args := []string{"pod", "create", "--name", pod}
	entries := links(cfg)
	if len(entries) == 0 {
		netspec := "pasta"
		if iface := firstInterface(cfg); iface != "" {
			netspec = "pasta:--interface," + iface
		}
		args = append(args, "--network", netspec)
		return append(args, publishArgs(cfg)...)
	}

	// A linked app needs its own egress too when it has non-link lists - a gateway app has
	// to reach the outside to be worth routing through, and a client that sends only some
	// destinations through a sibling still goes direct with the rest. pasta cannot do this:
	// podman refuses outright ("cannot set multiple networks without bridge network mode"),
	// so such an app is put on a bridge instead.
	//
	// Its OWN bridge, not the default one. Apps sharing a bridge can reach each other over
	// L2, which would leave isolation resting on the nft rules alone - and an app whose
	// egress list is an all-blacklist runs default-accept, so it would reach every other
	// app on that bridge. A bridge per app keeps them apart whatever their rules say.
	if needsOwnEgress(cfg) {
		args = append(args, "--network", EgressNetwork(cfg.AppNameID)+":interface_name="+egressIface)
	}
	for _, server := range cfg.NetworkMeta.DNSServers {
		// The app is handed these instead of the resolver podman would put on its link,
		// which on an --internal bridge answers sibling names and forwards nothing.
		args = append(args, "--dns", strings.TrimSpace(server))
	}
	if forwards(cfg) {
		// Set here because a container cannot set it itself: /proc/sys is read-only in the
		// namespace, so an app that agreed to route for its siblings would silently drop
		// every packet it was meant to forward.
		args = append(args, "--sysctl", "net.ipv4.ip_forward=1", "--sysctl", "net.ipv6.conf.all.forwarding=1")
	}
	// alias=<AppNameID>: podman resolves the network alias but NOT the pod's app
	// container name, so this makes each app reachable on the link at its AppNameID
	// (a consumer connects to "<producer>:<port>") instead of the pod name.
	for _, entry := range entries {
		args = append(args, "--network", entry.network+":interface_name="+entry.iface+",alias="+cfg.AppNameID)
	}
	return append(args, publishArgs(cfg)...)
}

// needsOwnEgress reports whether a linked app also carries networking that has to leave the
// private bridges - any list that is not itself a link.
func needsOwnEgress(cfg schema.AppConfig) bool {
	for _, netList := range cfg.NetworkMeta.NetworkLists {
		if !isLinkList(netList) {
			return true
		}
	}
	return false
}

// publishArgs maps tier-3 (LAN) ingress lists - Ingress && Host - onto pod `-p` port
// forwards so the LAN can reach the app's published ports; the nft input chain then
// restricts who (source CIDR) actually gets through, and pasta binds the pod's interface
// (firstInterface). Each port is forwarded for both tcp and udp to match the input
// chain; there is no host-port remap (published port == container port). Self-scoped
// ingress (tier 2) publishes nothing to the host and never reaches here - checkNetwork
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
// pod's netns. It carries only NET_ADMIN - namespaced to the pod's userns, so
// harmless on the host (section 5.3) - reads the ruleset from stdin, and exits.
func nftApplyArgs(pod, image string) []string {
	return []string{
		"run", "--pod", pod, "--rm", "-i", "--pull", "never",
		"--security-opt", "no-new-privileges", "--cap-drop", "all", "--cap-add", "NET_ADMIN",
		image, "nft", "-f", "-",
	}
}

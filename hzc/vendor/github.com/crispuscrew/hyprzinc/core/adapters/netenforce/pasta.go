// Package netenforce holds the NetEnforcer adapters — the swappable egress
// mechanisms. Each implements ports.NetEnforcer: how the app container attaches to
// the network (RunFlags), what must happen to establish and LOCK the netns before
// the app starts (Prepare), and how to tear it down (Teardown).
//
// Three ship today: None (no network), Container (join another container's netns,
// e.g. a VPN), and Pasta (the enforced egress allowlist via a pod + nftables). A
// future mechanism — eBPF egress, a proxy sidecar, an external traffic controller —
// is one more file here implementing the same interface; nothing in core/app or the
// podman runtime changes (docs/architecture.md §5.3, §13).
package netenforce

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/core/ports"
)

// DefaultNetfilterImage is the locally built helper (trusted-* per §5.5) that
// carries nft. It runs once per pasta launch to lock the pod's netns before the app
// starts. Build it with `make netfilter-image` (images/netfilter). The nft step
// runs it with --pull=never (see nftApplyArgs): the privileged helper is always the
// locally vetted build, never pulled from a registry, and a missing image fails
// fast with a clear error instead of a headless short-name prompt.
const DefaultNetfilterImage = "hyprzinc/trusted-netfilter:local"

// Pasta enforces an egress allowlist: the app runs inside a pod whose netns is
// locked down with an nftables ruleset before the app container ever starts (§5.3).
// It satisfies ports.NetEnforcer.
type Pasta struct{}

// PodName is the pod that owns a pasta app's filtered network namespace.
func PodName(app string) string { return app + "-pod" }

// RunFlags attaches the app container to the pasta pod. The pod's infra container
// owns networking and the nft ruleset is already in place (from Prepare), so the
// app only joins the locked netns — no per-app --network flag, no net caps.
func (Pasta) RunFlags(cfg domain.AppConfig) []string {
	return []string{"--pod", PodName(cfg.App.Name)}
}

// Prepare returns the two steps that guarantee no unfiltered-egress window (§5.3):
// create the pod (pasta netns), then lock the netns with nft *before any app
// starts*. The app run itself is appended by the caller (core/app) using RunFlags.
func (Pasta) Prepare(cfg domain.AppConfig, opt domain.HostOptions) []ports.Command {
	pod := PodName(cfg.App.Name)
	image := opt.NetfilterImage
	if image == "" {
		image = DefaultNetfilterImage
	}
	return []ports.Command{
		{Args: podCreateArgs(cfg, pod), Desc: "create pod (pasta netns)"},
		{Args: nftApplyArgs(pod, image), Stdin: NFTRuleset(cfg), Desc: "lock netns with nft (before app)"},
	}
}

// Teardown removes the pod, which owns the filtered netns — so the app and its
// firewall go away in one step (and no stale, rule-less netns is left behind).
func (Pasta) Teardown(cfg domain.AppConfig) []string {
	return []string{"pod", "rm", "-f", PodName(cfg.App.Name)}
}

// NFTRuleset renders the nftables ruleset enforcing a pasta app's egress allowlist
// (§5.3). Pure function over the validated config; the ruleset is loaded into the
// pod's own netns by the netfilter init step, before the app container starts — so
// the app never sees an open network.
//
// Policy: default-drop on output. Loopback and established/related return traffic
// are always allowed. Egress is permitted only to the listed CIDRs (and, when ports
// are listed, only on those ports). IPv6 with no ipv6_cidr is therefore blocked
// outright. block_dns drops 53/853 ahead of the allow rules, so DNS can never leak
// even through a broad CIDR grant (the in-tunnel resolver case is M4).
func NFTRuleset(cfg domain.AppConfig) string {
	var bld strings.Builder
	bld.WriteString("table inet hyprzinc {\n")
	bld.WriteString("\tchain output {\n")
	bld.WriteString("\t\ttype filter hook output priority 0; policy drop;\n")
	bld.WriteString("\t\toif \"lo\" accept\n")
	bld.WriteString("\t\tct state established,related accept\n")
	if cfg.Network.BlockDNS {
		bld.WriteString("\t\tudp dport { 53, 853 } drop\n")
		bld.WriteString("\t\ttcp dport { 53, 853 } drop\n")
	}
	writeAllow(&bld, "ip", cfg.Network.IPv4CIDR, cfg.Network.Ports)
	writeAllow(&bld, "ip6", cfg.Network.IPv6CIDR, cfg.Network.Ports)
	bld.WriteString("\t}\n")
	bld.WriteString("}\n")
	return bld.String()
}

// writeAllow emits the accept rules for one address family. No CIDRs → nothing
// (default-drop blocks the family). Ports listed → only those ports; otherwise all
// ports to the listed CIDRs.
func writeAllow(bld *strings.Builder, family string, cidrs []string, ports []int) {
	if len(cidrs) == 0 {
		return
	}
	daddr := family + " daddr { " + strings.Join(cidrs, ", ") + " }"
	if len(ports) == 0 {
		fmt.Fprintf(bld, "\t\t%s accept\n", daddr)
		return
	}
	portsList := portList(ports)
	fmt.Fprintf(bld, "\t\t%s tcp dport { %s } accept\n", daddr, portsList)
	fmt.Fprintf(bld, "\t\t%s udp dport { %s } accept\n", daddr, portsList)
}

func portList(ports []int) string {
	strs := make([]string, len(ports))
	for idx, port := range ports {
		strs[idx] = strconv.Itoa(port)
	}
	return strings.Join(strs, ", ")
}

// podCreateArgs builds `podman pod create` for a pasta app's filtered netns.
func podCreateArgs(cfg domain.AppConfig, pod string) []string {
	netspec := "pasta"
	if iface := strings.TrimSpace(cfg.Network.Interface); iface != "" {
		// pasta copies addressing from this host interface (§3 network.interface).
		netspec = "pasta:--interface," + iface
	}
	return []string{"pod", "create", "--name", pod, "--network", netspec}
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

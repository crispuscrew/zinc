package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

// startDependencies brings up everything cfg needs before cfg itself launches (docs
// section 6.6: "auto-starts dependencies first"). Each name in StartConditions.DependsOn
// that is not already running is loaded from the store and launched first,
// depth-first, so a dependency's own dependencies come up before it. An
// already-running dependency is left untouched. A dependency cycle is reported as an
// error rather than recursed into forever.
//
// chain is the stack of apps currently mid-launch (root → cfg's parent); cfg is
// appended before recursing, so a name reappearing in it is a cycle.
func (svc Service) startDependencies(cfg schema.AppConfig, opt options.HostOptions, chain []string, started map[string]bool) error {
	if len(cfg.StartConditions.DependsOn) == 0 {
		return nil
	}
	// Three-index slice caps chain so append allocates a fresh backing array rather
	// than aliasing a sibling recursion's storage.
	chain = append(chain[:len(chain):len(chain)], cfg.AppNameID)
	running, err := svc.runtime.Running()
	if err != nil {
		return fmt.Errorf("%s: checking running containers before starting dependencies: %w", cfg.AppNameID, err)
	}
	if running == nil {
		running = map[string]bool{}
	}
	for _, dep := range cfg.StartConditions.DependsOn {
		if running[dep] {
			continue // already up - leave it as-is
		}
		if idx := slices.Index(chain, dep); idx >= 0 {
			return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(chain[idx:], " -> "), dep)
		}
		depCfg, err := svc.store.Load(dep)
		if err != nil {
			return fmt.Errorf("%s depends on %q: %w", cfg.AppNameID, dep, err)
		}
		if err := svc.launch(depCfg, opt, chain, started); err != nil {
			return fmt.Errorf("starting dependency %q of %s: %w", dep, cfg.AppNameID, err)
		}
		running[dep] = true // so a name listed twice is not started twice
	}
	return nil
}

// checkNetwork fails closed on NetworkLists this build cannot enforce yet. Supported:
// self-scoped egress allow/deny lists (own pasta netns + nft output chain, section 5.3), tier-3
// LAN publishing (Ingress && Host - nft input chain + pod `-p`), and tier-2 sibling links
// (a producer's self-scoped ingress, a consumer's egress naming its AppName - a private
// interface-gated bridge). Rejected so a config is stopped at launch rather than silently
// mis-enforced: a routing gateway (multi-homing), an ingress list that targets an AppName
// (contradictory), and host-scoped egress.
//
// Links may now coexist with other networking on one app. They could not before, because
// the ruleset was one kind or the other and whichever ran ignored the other kind of list
// outright - a linked app's address rules simply vanished. The renderer gates both at once
// now, so the combination is enforceable and no longer refused: a gateway app needs a link
// AND real egress to be worth routing through.
func checkNetwork(cfg schema.AppConfig) error {
	for index, netList := range cfg.NetworkMeta.NetworkLists {
		appName := strings.TrimSpace(netList.AppName)
		switch {
		case netList.GatewayV4 != "" || netList.GatewayV6 != "":
			return fmt.Errorf("%s: NetworkLists[%d]: routing through a gateway (multi-homing) is not supported in this build yet", cfg.AppNameID, index)
		case netList.Ingress && appName != "":
			return fmt.Errorf("%s: NetworkLists[%d]: an ingress list cannot target an AppName - a producer publishes to any sibling that joins its link, and the consumer names the producer", cfg.AppNameID, index)
		case netList.Host && !netList.Ingress:
			return fmt.Errorf("%s: NetworkLists[%d]: host-scoped egress is not supported in this build yet", cfg.AppNameID, index)
		case isLinkList(netList) && netList.Blacklist:
			return fmt.Errorf("%s: NetworkLists[%d]: a sibling link list cannot be a blacklist - its Ports are the allowed set and a blacklist would open them instead of gating them", cfg.AppNameID, index)
		}
	}
	return nil
}

// isLinkList reports whether a NetworkList is a tier-2 sibling link: a producer's
// self-scoped ingress (Ingress, no Host, no AppName) or a consumer's sibling egress
// (egress, no Host, an AppName).
func isLinkList(netList schema.NetworkList) bool {
	appName := strings.TrimSpace(netList.AppName)
	producer := netList.Ingress && !netList.Host && appName == ""
	consumer := !netList.Ingress && !netList.Host && appName != ""
	return producer || consumer
}

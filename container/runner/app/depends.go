package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

// startDependencies brings up everything cfg needs before cfg itself launches (docs
// §6.6: "auto-starts dependencies first"). Each name in StartConditions.DependsOn
// that is not already running is loaded from the store and launched first,
// depth-first, so a dependency's own dependencies come up before it. An
// already-running dependency is left untouched. A dependency cycle is reported as an
// error rather than recursed into forever.
//
// chain is the stack of apps currently mid-launch (root → cfg's parent); cfg is
// appended before recursing, so a name reappearing in it is a cycle.
func (svc Service) startDependencies(cfg schema.AppConfig, opt options.HostOptions, chain []string) error {
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
			continue // already up — leave it as-is
		}
		if idx := slices.Index(chain, dep); idx >= 0 {
			return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(chain[idx:], " -> "), dep)
		}
		depCfg, err := svc.store.Load(dep)
		if err != nil {
			return fmt.Errorf("%s depends on %q: %w", cfg.AppNameID, dep, err)
		}
		if err := svc.launch(depCfg, opt, chain); err != nil {
			return fmt.Errorf("starting dependency %q of %s: %w", dep, cfg.AppNameID, err)
		}
		running[dep] = true // so a name listed twice is not started twice
	}
	return nil
}

// checkNetwork fails closed on NetworkLists this build cannot enforce yet. Supported:
// self-scoped egress allow/deny lists (own pasta netns + nft output chain, §5.3) and
// tier-3 LAN publishing (Ingress && Host — nft input chain + pod `-p`). Deferred and
// rejected here so a config using them is stopped at launch rather than silently
// mis-enforced: self-scoped ingress (tier-2 sibling reach), host-scoped egress, a
// sibling AppName link, and a routing gateway (multi-homing).
func checkNetwork(cfg schema.AppConfig) error {
	for index, netList := range cfg.NetworkMeta.NetworkLists {
		switch {
		case netList.GatewayV4 != "" || netList.GatewayV6 != "":
			return fmt.Errorf("%s: NetworkLists[%d]: routing through a gateway (multi-homing) is not supported in this build yet", cfg.AppNameID, index)
		case netList.Ingress && netList.Host:
			// tier-3: publish ports to the LAN. Supported.
		case netList.Ingress:
			return fmt.Errorf("%s: NetworkLists[%d]: publishing to a sibling app (self-scoped Ingress) is not supported in this build yet", cfg.AppNameID, index)
		case netList.Host:
			return fmt.Errorf("%s: NetworkLists[%d]: host-scoped egress is not supported in this build yet", cfg.AppNameID, index)
		case strings.TrimSpace(netList.AppName) != "":
			return fmt.Errorf("%s: NetworkLists[%d]: sharing a sibling app's network (AppName %q) is not supported in this build yet", cfg.AppNameID, index, netList.AppName)
		}
	}
	return nil
}

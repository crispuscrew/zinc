package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

// startDependencies brings up everything cfg needs before cfg itself launches (docs
// section 6.6: "auto-starts dependencies first"). Each name in StartConditions.DependsOn
// that is not already running is loaded from the store and launched first,
// depth-first, so a dependency's own dependencies come up before it, and then - if it
// declares a ReadyCheck - waited for until it says it is ready. An already-running
// dependency is left untouched. A dependency cycle is reported as an error rather than
// recursed into forever.
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
		depCfg, err := svc.store.LoadResolved(dep)
		if err != nil {
			return fmt.Errorf("%s depends on %q: %w", cfg.AppNameID, dep, err)
		}
		if err := svc.launch(depCfg, opt, chain, started); err != nil {
			return fmt.Errorf("starting dependency %q of %s: %w", dep, cfg.AppNameID, err)
		}
		running[dep] = true // so a name listed twice is not started twice
		if err := svc.waitReady(depCfg, cfg.AppNameID); err != nil {
			return err
		}
	}
	return nil
}

// readyPollInterval is the gap between readiness probes. Each probe execs into the
// dependency's container, so this trades a little startup latency against not hammering a
// container that is busy doing the very thing being waited for.
var readyPollInterval = 500 * time.Millisecond

// defaultReadyTimeout bounds a wait whose app did not set StartConditions.ReadyTimeoutSec.
// Long enough for a VPN handshake over a slow link, short enough that a dependency which is
// never going to be ready fails the launch with a message instead of hanging.
const defaultReadyTimeout = 60 * time.Second

// waitReady holds the launch of dependent until depCfg reports itself ready, and fails the
// launch if it does not within its timeout. An app with no ReadyCheck is ready as soon as it
// is running, which is what DependsOn meant before and still means for most apps.
//
// The failure is deliberately fatal rather than a warning that lets the dependent start
// anyway. The case this exists for is a gateway: a client routed through a sibling has that
// sibling as its default route and its DNS, so starting it before the tunnel is up gives it
// no working network at all - fail closed and say which dependency was not ready, rather
// than start an app whose every connection will fail for a reason nothing reported.
//
// Only a dependency this launch started is waited on. One that was already running was
// either gated the same way by whoever started it, or started by hand, and re-probing it
// would turn every launch behind a momentarily-unhealthy dependency into a failure rather
// than the start-order race this closes.
//
// A dependency that failed to become ready is left running, like every other dependency an
// aborted launch had already started. Its logs are how an author finds out why it never came
// up, and stopping it would also stop it for anything else already routed through it.
func (svc Service) waitReady(depCfg schema.AppConfig, dependent string) error {
	if len(depCfg.StartConditions.ReadyCheck) == 0 {
		return nil
	}
	timeout := defaultReadyTimeout
	if seconds := depCfg.StartConditions.ReadyTimeoutSec; seconds > 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		err := svc.runtime.HealthProbe(depCfg.AppNameID)
		if err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%s: dependency %q was not ready within %s: %w",
				dependent, depCfg.AppNameID, timeout, err)
		}
		time.Sleep(readyPollInterval)
	}
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

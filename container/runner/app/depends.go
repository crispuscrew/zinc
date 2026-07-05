package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/crispuscrew/hyprzinc/core/domain"
)

// startDependencies brings up everything cfg needs before cfg itself launches
// (docs §6.6: "auto-starts dependencies first"). Each name in depends_on.containers
// that is not already running is loaded from the store and launched first,
// depth-first, so a dependency's own dependencies come up before it. An
// already-running dependency is left untouched. A dependency cycle is reported as an
// error rather than recursed into forever.
//
// chain is the stack of apps currently mid-launch (root → cfg's parent); cfg is
// appended before recursing, so a name reappearing in it is a cycle.
func (svc Service) startDependencies(cfg domain.AppConfig, opt domain.HostOptions, chain []string) error {
	if len(cfg.DependsOn.Containers) == 0 {
		return nil
	}
	// Three-index slice caps chain so append allocates a fresh backing array rather
	// than aliasing a sibling recursion's storage.
	chain = append(chain[:len(chain):len(chain)], cfg.App.Name)
	running, err := svc.runtime.Running()
	if err != nil {
		return fmt.Errorf("%s: checking running containers before starting dependencies: %w", cfg.App.Name, err)
	}
	if running == nil {
		running = map[string]bool{}
	}
	for _, dep := range cfg.DependsOn.Containers {
		if running[dep] {
			continue // already up — leave it as-is
		}
		if idx := slices.Index(chain, dep); idx >= 0 {
			return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(chain[idx:], " -> "), dep)
		}
		depCfg, err := svc.store.Load(dep)
		if err != nil {
			return fmt.Errorf("%s depends on %q: %w", cfg.App.Name, dep, err)
		}
		if err := svc.launch(depCfg, opt, chain); err != nil {
			return fmt.Errorf("starting dependency %q of %s: %w", dep, cfg.App.Name, err)
		}
		running[dep] = true // so a name listed twice is not started twice
	}
	return nil
}

// verifyNetTarget fails closed when a container-mode app's network target (the
// container whose netns it shares — the VPN container, §6.4) will not be present at
// launch. If the target is a declared dependency, startDependencies has already
// ensured it; otherwise it must already be running, or attaching would share a
// non-existent namespace (and risk a silent fall-through to the host network). This
// is the launch-time existence check domain.Validate explicitly defers here (§6.6).
func (svc Service) verifyNetTarget(cfg domain.AppConfig) error {
	if cfg.Network.Mode != domain.NetworkContainer {
		return nil
	}
	target := cfg.Network.Target
	if slices.Contains(cfg.DependsOn.Containers, target) {
		return nil // depends_on ensured it (started above, or already running)
	}
	running, err := svc.runtime.Running()
	if err != nil {
		return fmt.Errorf("%s: checking network target %q: %w", cfg.App.Name, target, err)
	}
	if !running[target] {
		return fmt.Errorf("%s: network target %q is not running and is not listed in depends_on.containers; add it there so it auto-starts first (§6.4, §6.6)", cfg.App.Name, target)
	}
	return nil
}

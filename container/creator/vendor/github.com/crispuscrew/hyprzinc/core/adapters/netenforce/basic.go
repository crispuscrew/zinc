package netenforce

import (
	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/core/ports"
)

// Compile-time checks that the enforcers satisfy ports.NetEnforcer.
var (
	_ ports.NetEnforcer = None{}
	_ ports.NetEnforcer = Container{}
	_ ports.NetEnforcer = Pasta{}
)

// None is the no-network enforcer: the app gets `--network none` and there is
// nothing to prepare or tear down beyond stopping the container. It satisfies
// ports.NetEnforcer.
type None struct{}

func (None) RunFlags(domain.AppConfig) []string                           { return []string{"--network", "none"} }
func (None) Prepare(domain.AppConfig, domain.HostOptions) []ports.Command { return nil }
func (None) Teardown(cfg domain.AppConfig) []string                       { return []string{"stop", cfg.App.Name} }

// Container joins another container's network namespace (e.g. a VPN container),
// `--network container:<target>` (§6.4). The target's lifecycle is its own; here
// there is nothing to prepare, and teardown just stops the app. Satisfies
// ports.NetEnforcer.
type Container struct{}

func (Container) RunFlags(cfg domain.AppConfig) []string {
	return []string{"--network", "container:" + cfg.Network.Target}
}
func (Container) Prepare(domain.AppConfig, domain.HostOptions) []ports.Command { return nil }
func (Container) Teardown(cfg domain.AppConfig) []string                       { return []string{"stop", cfg.App.Name} }

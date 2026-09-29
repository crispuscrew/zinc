package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func (svc Service) startDependencies(cfg schema.AppConfig, opt options.HostOptions, chain []string, started map[string]bool) error {
	if len(cfg.StartConditions.DependsOn) == 0 {
		return nil
	}
	chain = append(chain[:len(chain):len(chain)], cfg.AppNameID)
	running, err := svc.runtime.Running()
	if err != nil {
		return fmt.Errorf("%s: checking dependencies: %w", cfg.AppNameID, err)
	}
	if running == nil {
		running = map[string]bool{}
	}
	for _, dependency := range cfg.StartConditions.DependsOn {
		if running[dependency] {
			continue
		}
		if index := slices.Index(chain, dependency); index >= 0 {
			return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(chain[index:], " -> "), dependency)
		}
		definition, err := svc.store.LoadResolved(dependency)
		if err != nil {
			return fmt.Errorf("%s depends on %q: %w", cfg.AppNameID, dependency, err)
		}
		if err := svc.launch(definition, opt, chain, started); err != nil {
			return fmt.Errorf("starting dependency %q of %s: %w", dependency, cfg.AppNameID, err)
		}
		running[dependency] = true
	}
	return nil
}

package app

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
)

func (svc Service) checkDependencies(cfg schema.AppConfig, chain []string, seen map[string]bool) error {
	if slices.Contains(chain, cfg.AppNameID) {
		return fmt.Errorf("dependency cycle: %s -> %s", strings.Join(chain, " -> "), cfg.AppNameID)
	}
	if seen[cfg.AppNameID] {
		return nil
	}
	if len(seen) >= 64 {
		return fmt.Errorf("dependency graph exceeds 64 apps")
	}
	seen[cfg.AppNameID] = true
	chain = append(slices.Clone(chain), cfg.AppNameID)
	for _, name := range cfg.StartConditions.DependsOn {
		if svc.Store == nil {
			return fmt.Errorf("dependency %s: no app store configured", name)
		}
		dependency, err := svc.Store.LoadResolved(name)
		if err != nil {
			return err
		}
		if err := validate.Validate(dependency); err != nil {
			return fmt.Errorf("dependency %s: %w", name, err)
		}
		if err := svc.checkDependencies(dependency, chain, seen); err != nil {
			return err
		}
	}
	return nil
}

func (svc Service) startDependencies(cfg schema.AppConfig) error {
	for _, name := range cfg.StartConditions.DependsOn {
		dependency, err := svc.Store.LoadResolved(name)
		if err != nil {
			return err
		}
		binary, args := "zcr", []string{"run", name, "--exec"}
		if dependency.Type == schema.ZincVirtualization {
			state, err := svc.State(name)
			if err != nil {
				return err
			}
			if state.Alive {
				continue
			}
			binary, err = os.Executable()
			if err != nil {
				return err
			}
			args = []string{"run", name}
		} else {
			output, err := exec.Command("zcr", "ps").Output()
			if err != nil {
				return fmt.Errorf("inspect dependency %s: %w", name, err)
			}
			if slices.Contains(strings.Fields(string(output)), name) {
				continue
			}
		}
		command := exec.Command(binary, args...)
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("start dependency %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

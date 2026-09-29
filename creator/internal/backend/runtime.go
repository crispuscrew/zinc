package backend

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/advisory"
	"github.com/crispuscrew/zinc/creator/internal/runner"
)

// Action is the type-aware UI path. CLI passthrough retains terminal ownership.
func (svc Service) Action(verb, name string, shell bool) (string, error) {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasSuffix(name, ".yaml") || strings.Contains(name, "/") {
		return "", fmt.Errorf("invalid store key %q", name)
	}
	cfg, err := svc.LoadResolved(name)
	if err != nil {
		return "", err
	}
	binary, args, err := actionArgs(cfg.Type, verb, name, shell)
	if err != nil {
		return "", err
	}
	stdout, warnings, err := runner.CaptureTo(binary, args...)
	if err != nil {
		return "", err
	}
	if verb == "run" || verb == "term" || verb == "build" || verb == "plan" {
		for _, warning := range advisory.Warnings(cfg) {
			warnings += "\nwarning: " + warning
		}
	}
	return strings.TrimSpace(warnings + "\n" + stdout), nil
}

func actionArgs(kind schema.Type, verb, name string, shell bool) (string, []string, error) {
	if kind == schema.ZincContainer {
		args := []string{verb, name}
		switch verb {
		case "run":
			args = append(args, "--exec")
		case "plan":
			args[0] = "run"
		case "term":
			if shell {
				args = append(args, "--shell")
			}
		}
		return runner.Binary, args, nil
	}
	if kind != schema.ZincVirtualization {
		return "", nil, fmt.Errorf("unknown app type %q", kind)
	}
	switch verb {
	case "run":
		return runner.VMBinary, []string{"run", name}, nil
	case "plan":
		return runner.VMBinary, []string{"run", name, "--dry-run"}, nil
	case "stop":
		return runner.VMBinary, []string{"stop", name}, nil
	case "term":
		return runner.VMBinary, []string{"console", name}, nil
	case "build":
		return "", nil, fmt.Errorf("%s is a VM app: no image to build", name)
	case "logs":
		return "", nil, fmt.Errorf("%s is a VM app: no container log; use its console", name)
	default:
		return "", nil, fmt.Errorf("%s is not supported for VM apps", verb)
	}
}

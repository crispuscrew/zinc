// Package podman implements the container runtime and image ports using direct argv.
package podman

import (
	"fmt"
	"slices"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/session"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

const (
	ctrXDGRuntime = "/run/zinc"
	ctrThemeDir   = "/etc/zinc/theme"
)

type Runtime struct{}

var (
	_ ports.Runtime       = Runtime{}
	_ ports.ImageBuilder  = Builder{}
	_ ports.ImageResolver = Resolver{}
)

type runMode int

const (
	modeForeground runMode = iota
	modeBackground
	modeTerminal
	modeHolder
)

func modeFor(cfg schema.AppConfig) runMode {
	switch {
	case cfg.StartConditions.Attached:
		return modeHolder
	case cfg.StartConditions.Terminal:
		return modeTerminal
	case cfg.StopConditions.Background:
		return modeBackground
	default:
		return modeForeground
	}
}

func lifecycleArgs(cfg schema.AppConfig) []string {
	args := []string{"run"}
	keep := cfg.StopConditions.KeepAlive || cfg.StopConditions.Autorestart
	switch modeFor(cfg) {
	case modeTerminal:
		if !keep {
			args = append(args, "--rm")
		}
		args = append(args, "-it")
	case modeBackground:
		args = append(args, "-d")
	case modeHolder:
		args = append(args, "-d")
		if !keep {
			args = append(args, "--rm")
		}
		args = append(args, "--init")
	default:
		if !keep {
			args = append(args, "--rm")
		}
	}
	if cfg.StopConditions.Autorestart {
		args = append(args, "--restart", "on-failure")
	}
	return append(args, "--pull", "never", "--name", cfg.AppNameID,
		"--security-opt", "no-new-privileges", "--cap-drop", "all")
}

// AppRunArgs applies structured settings first, then the user's raw backend argv.
func (Runtime) AppRunArgs(cfg schema.AppConfig, opt options.HostOptions, netFlags []string) ([]string, error) {
	for index, arg := range cfg.RunnerFlags {
		if strings.ContainsRune(arg, '\x00') {
			return nil, fmt.Errorf("RunnerFlags[%d]: NUL in argv", index)
		}
	}
	args := lifecycleArgs(cfg)
	inPod := slices.Contains(netFlags, "--pod")
	args = append(args, userArgs(cfg.InternalUserMeta, inPod)...)
	args = append(args, resourceArgs(cfg.ResourcesMeta)...)
	args = append(args, session.EnvArgs(cfg.StartConditions.EntrypointEnv)...)
	args = append(args, netFlags...)
	if cfg.MinimizeFingerprint {
		args = append(args, "--unsetenv=container")
		// A pod owns the shared UTS namespace; its creator sets the hostname there.
		if !inPod {
			args = append(args, "--hostname", "localhost")
		}
	}
	if cfg.StartConditions.ReadOnlyRootfs {
		args = append(args, "--read-only")
	}
	sockets, err := socketArgs(cfg, opt)
	if err != nil {
		return nil, err
	}
	args = append(args, sockets...)
	mounts, err := mountArgs(cfg, opt)
	if err != nil {
		return nil, err
	}
	args = append(args, mounts...)
	if modeFor(cfg) == modeHolder {
		// Clear the image ENTRYPOINT: the holder must not invoke the app accidentally.
		args = append(args, "--entrypoint", "[]")
	} else if strings.TrimSpace(cfg.StartConditions.Entrypoint) != "" {
		args = append(args, "--entrypoint", `["/bin/sh","-c"]`)
	}
	args = append(args, cfg.RunnerFlags...)
	args = append(args, derived.RunImage(cfg))
	if modeFor(cfg) == modeHolder {
		args = append(args, HolderCmd()...)
	} else if strings.TrimSpace(cfg.StartConditions.Entrypoint) != "" {
		args = append(args, cfg.StartConditions.Entrypoint)
	}
	return args, nil
}

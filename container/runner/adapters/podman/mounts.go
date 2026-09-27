package podman

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func mountArgs(cfg schema.AppConfig, opt options.HostOptions) ([]string, error) {
	var args []string
	if !cfg.DisplayMeta.DisableGpuAccess {
		args = append(args, "--device", "/dev/dri")
	}
	if cfg.HostTheme && opt.ThemeBundleDir != "" {
		args = append(args, "-v", opt.ThemeBundleDir+":"+ctrThemeDir+":ro")
	}
	for _, config := range cfg.Configs {
		if opt.BundleDir == "" {
			return nil, fmt.Errorf("%s: no bundle directory resolved for Configs[%q]", cfg.AppNameID, config.BundlePath)
		}
		mode := "ro,noexec"
		if config.Writable {
			mode = "rw,noexec"
		}
		args = append(args, "-v", filepath.Join(opt.BundleDir, config.BundlePath)+":"+config.InnerMount+":"+mode)
	}
	for _, volume := range cfg.Volumes {
		if !volume.HostMounted || strings.TrimSpace(volume.HostMount) == "" {
			args = append(args, "--mount", tmpfsMount(volume))
			continue
		}
		mode := "ro"
		if volume.Writable {
			mode = "rw"
		}
		if volume.Executable {
			mode += ",exec"
		} else {
			mode += ",noexec"
		}
		args = append(args, "-v", volume.HostMount+":"+volume.InnerMount+":"+mode)
	}
	home := opt.HomeDir
	if home == "" {
		home = "/root"
	}
	if user := cfg.InternalUserMeta; user.UseNonRootUser && user.NonRootUserName != "" {
		home = "/home/" + user.NonRootUserName
	}
	for _, key := range cfg.Keys {
		directory := ".ssh"
		if key.Type == schema.GPG {
			directory = ".gnupg"
		}
		args = append(args, "-v", key.Path+":"+filepath.Join(home, directory, filepath.Base(key.Path))+":ro")
	}
	return args, nil
}

func tmpfsMount(volume schema.Volume) string {
	opts := []string{"type=tmpfs", "destination=" + volume.InnerMount, "nosuid", "nodev"}
	if !volume.Writable {
		opts = append(opts, "ro")
	}
	if !volume.Executable {
		opts = append(opts, "noexec")
	}
	if volume.SizeLimited {
		opts = append(opts, fmt.Sprintf("tmpfs-size=%dm", volume.SizeLimitMiB))
	}
	return strings.Join(opts, ",")
}

func userArgs(user schema.InternalUserMeta, inPod bool) []string {
	var args []string
	if user.KeepUserID && !inPod {
		args = append(args, "--userns=keep-id")
	}
	if user.UseNonRootUser && user.NonRootUserName != "" {
		args = append(args, "--user", user.NonRootUserName)
	}
	return args
}

func resourceArgs(resources schema.ResourcesMeta) []string {
	var args []string
	if resources.MaxCPUCores > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(resources.MaxCPUCores, 'f', -1, 64))
	}
	if resources.MaxRamMiB > 0 {
		args = append(args, "--memory", strconv.FormatInt(resources.MaxRamMiB, 10)+"m")
	}
	if resources.PIDsLimit > 0 {
		args = append(args, "--pids-limit", strconv.FormatInt(resources.PIDsLimit, 10))
	}
	return args
}

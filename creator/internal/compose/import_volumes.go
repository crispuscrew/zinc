package compose

import (
	"github.com/crispuscrew/zinc/common/domain/schema"
	"strings"
)

func importVolumes(service Service, note func(string, ...any)) []schema.Volume {
	var volumes []schema.Volume
	for _, mount := range service.Volumes {
		parts := strings.Split(mount, ":")
		if len(parts) < 2 {
			note("volume %q was dropped: no host path", mount)
			continue
		}
		host, inner := parts[0], parts[1]
		if !strings.HasPrefix(host, "/") {
			if strings.HasPrefix(host, ".") || strings.HasPrefix(host, "~") {
				note("volume %q was dropped: relative to the compose file or shell home; supply an absolute path", mount)
			} else {
				note("volume %q was dropped: named compose volume has no host path", mount)
			}
			continue
		}
		volume := schema.Volume{HostMounted: true, HostMount: host, InnerMount: inner}
		explicit := false
		if len(parts) > 2 {
			for _, option := range strings.Split(parts[2], ",") {
				switch strings.TrimSpace(option) {
				case "rw":
					volume.Writable, explicit = true, true
				case "ro":
					explicit = true
				case "exec":
					volume.Executable = true
				case "noexec":
					volume.Executable = false
				default:
					note("volume %q option %q was not imported", mount, option)
				}
			}
		}
		if !explicit {
			note("volume %q became read-only and noexec unless explicitly requested otherwise", mount)
		}
		volumes = append(volumes, volume)
	}
	return volumes
}

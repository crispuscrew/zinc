package compose

import (
	"path/filepath"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func volumeMounts(cfg schema.AppConfig, note func(string, ...any)) StringList {
	var mounts StringList
	for index, volume := range cfg.Volumes {
		if !volume.HostMounted || strings.TrimSpace(volume.HostMount) == "" {
			note("Volumes[%d] (%s) is not represented: no host bind source", index, volume.InnerMount)
			continue
		}
		options := "ro"
		if volume.Writable {
			options = "rw"
		}
		if volume.Executable {
			options += ",exec"
		} else {
			options += ",noexec"
		}
		mounts = append(mounts, volume.HostMount+":"+volume.InnerMount+":"+options)
	}
	home := "/root"
	if user := cfg.InternalUserMeta; user.UseNonRootUser && user.NonRootUserName != "" {
		home = "/home/" + user.NonRootUserName
	}
	for _, key := range cfg.Keys {
		directory := ".ssh"
		if key.Type == schema.GPG {
			directory = ".gnupg"
		}
		mounts = append(mounts, key.Path+":"+filepath.Join(home, directory, filepath.Base(key.Path))+":ro")
	}
	return mounts
}

package validate

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func checkVirtualization(cfg schema.AppConfig, add addFunc) {
	checkBaseImage(cfg.ImageMeta.Image, add)
	resources := cfg.ResourcesMeta
	if resources.MaxRamMiB <= 0 {
		add("ResourcesMeta.MaxRamMiB %d: must be > 0 for a VM", resources.MaxRamMiB)
	}
	if !math.IsNaN(resources.MaxCPUCores) && !math.IsInf(resources.MaxCPUCores, 0) {
		if resources.MaxCPUCores <= 0 {
			add("ResourcesMeta.MaxCPUCores %v: must be > 0 for a VM", resources.MaxCPUCores)
		} else if resources.MaxCPUCores != math.Trunc(resources.MaxCPUCores) {
			add("ResourcesMeta.MaxCPUCores %v: must be a whole number for a VM", resources.MaxCPUCores)
		}
	}
	if resources.PIDsLimit != 0 {
		add("ResourcesMeta.PIDsLimit: not supported for a VM app (the guest kernel owns its process table)")
	}
	checkCloudInit(cfg, add)
}

// This path is also embedded in qemu drive properties. File identity/pins belong
// to runtime options; canonical here means lexical, without filesystem I/O.
func checkBaseImage(image string, add addFunc) {
	switch {
	case image == "":
		add("ImageMeta.Image: must not be empty (a VM app needs a base disk image)")
	case hasUnsafe(image) || strings.ContainsRune(image, ','):
		add("ImageMeta.Image %q: must not contain whitespace, control characters or ',' (qemu drive property separator)", image)
	case !filepath.IsAbs(image):
		add("ImageMeta.Image %q: must be an absolute path for a VM app", image)
	case image == "/" || filepath.Clean(image) != image:
		add("ImageMeta.Image %q: must be a canonical base disk path with no '.', '..', repeated or trailing separators", image)
	}
}

func checkCloudInit(cfg schema.AppConfig, add addFunc) {
	image := cfg.ImageMeta
	if !image.CloudInit && (image.PublicSSHKeyPath != "" || len(image.Install) > 0 || cfg.InternalUserMeta.UseNonRootUser) {
		add("ImageMeta.CloudInit: required for PublicSSHKeyPath, Install and guest user creation")
	}
	path := image.PublicSSHKeyPath
	switch {
	case path == "":
	case hasUnsafe(path):
		add("ImageMeta.PublicSSHKeyPath %q: must not contain whitespace or control characters", path)
	case !filepath.IsAbs(path) || filepath.Clean(path) != path:
		add("ImageMeta.PublicSSHKeyPath %q: must be a canonical absolute path", path)
	case !strings.HasSuffix(path, ".pub"):
		add("ImageMeta.PublicSSHKeyPath %q: must be a PUBLIC key (.pub); the guest can read the seed", path)
	}
}

func checkVMFieldsUnset(cfg schema.AppConfig, add addFunc) {
	for _, field := range []struct {
		set  bool
		name string
	}{
		{cfg.StartConditions.LoaderBIOS, "StartConditions.LoaderBIOS"},
		{cfg.StartConditions.SecureBoot, "StartConditions.SecureBoot"},
		{cfg.StartConditions.TPM, "StartConditions.TPM"},
		{cfg.ImageMeta.CloudInit, "ImageMeta.CloudInit"},
		{cfg.ImageMeta.PublicSSHKeyPath != "", "ImageMeta.PublicSSHKeyPath"},
	} {
		if field.set {
			add("%s: only applies to a VM app", field.name)
		}
	}
}

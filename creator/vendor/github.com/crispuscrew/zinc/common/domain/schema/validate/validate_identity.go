package validate

import (
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func checkIdentity(cfg schema.AppConfig, add addFunc) {
	if cfg.SchemaVersion != schema.SchemaVersion {
		add("SchemaVersion: got %d, want %d", cfg.SchemaVersion, schema.SchemaVersion)
	}
	switch cfg.Type {
	case schema.ZincContainer, schema.ZincVirtualization:
	default:
		add("Type %q: must be %s or %s", cfg.Type, schema.ZincContainer, schema.ZincVirtualization)
	}
	if err := AppName(cfg.AppNameID); err != nil {
		add("%s", err)
	}
	if cfg.Inherits != "" {
		if !nameRE.MatchString(cfg.Inherits) {
			add("Inherits %q: must be lowercase [a-z0-9._-] starting alphanumeric", cfg.Inherits)
		} else if cfg.Inherits == cfg.AppNameID {
			add("Inherits %q: an app cannot inherit from itself", cfg.Inherits)
		}
	}
	if name := cfg.InternalUserMeta.NonRootUserName; name != "" && !nameRE.MatchString(name) {
		add("InternalUserMeta.NonRootUserName %q: must be lowercase [a-z0-9._-] starting alphanumeric", name)
	}
}

func checkInternalUser(user schema.InternalUserMeta, add addFunc) {
	if user.UseNonRootUser && strings.TrimSpace(user.NonRootUserName) == "" {
		add("InternalUserMeta.UseNonRootUser: set NonRootUserName too; the named user must exist in the image")
	}
	if !user.UseNonRootUser && user.NonRootUserName != "" {
		add("InternalUserMeta.NonRootUserName %q: has no effect without UseNonRootUser", user.NonRootUserName)
	}
}

func checkContainerImage(image string, add addFunc) {
	switch {
	case strings.TrimSpace(image) == "":
		add("ImageMeta.Image: must not be empty")
	case hasUnsafe(image):
		add("ImageMeta.Image %q: must be a single-line reference (no whitespace or control characters)", image)
	case !LocalImage(image) && !digestRE.MatchString(image):
		add("ImageMeta.Image %q: third-party images must be digest-pinned (...@sha256:<64 hex>); only localhost/ images may use a mutable tag", image)
	}
}

// Install entries form RUN directives for containers and runcmd entries for VMs.
func checkInstall(install []string, add addFunc) {
	for index, step := range install {
		if hasControl(step) {
			add("ImageMeta.Install[%d]: must not contain control characters (a newline would inject extra Containerfile directives)", index)
		}
	}
}

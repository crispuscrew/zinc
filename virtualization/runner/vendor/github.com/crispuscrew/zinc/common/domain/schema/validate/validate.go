package validate

import (
	"errors"
	"fmt"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Validate is pure: it checks schema values, not host devices or other app configs.
// Runtime enforcement must resolve those references before granting access.
func Validate(cfg schema.AppConfig) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	checkIdentity(cfg, add)
	checkLifecycle(cfg, add)
	checkInstall(cfg.ImageMeta.Install, add)
	checkResources(cfg.ResourcesMeta, add)
	checkInternalUser(cfg.InternalUserMeta, add)
	checkNetwork(cfg.NetworkMeta, add)
	checkAudio(cfg, add)
	checkDisplay(cfg, add)
	checkEnv("StartConditions.EntrypointEnv", cfg.StartConditions.EntrypointEnv, add)
	checkEnv("StartConditions.AttachedEnv", cfg.StartConditions.AttachedEnv, add)
	checkRawFlags("CreatorFlags", cfg.CreatorFlags, add)
	checkRawFlags("RunnerFlags", cfg.RunnerFlags, add)
	checkNotifications(cfg, add)
	checkDBus(cfg, add)
	checkSourceTag(cfg.ImageMeta.SourceTag, add)
	for index, volume := range cfg.Volumes {
		checkVolume(index, volume, add)
	}
	for index, config := range cfg.Configs {
		checkConfig(index, config, add)
	}
	checkKeys(cfg.Keys, add)
	if cfg.Type == schema.ZincVirtualization {
		checkVirtualization(cfg, add)
		checkGuestUnsupported(cfg, add)
	} else if cfg.Type == schema.ZincContainer {
		checkContainerImage(cfg.ImageMeta.Image, add)
		checkVMFieldsUnset(cfg, add)
	}
	return errors.Join(errs...)
}

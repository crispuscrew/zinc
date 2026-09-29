package validate

import (
	"math"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func checkLifecycle(cfg schema.AppConfig, add addFunc) {
	start := cfg.StartConditions
	for index, dependency := range start.DependsOn {
		if !nameRE.MatchString(dependency) {
			add("StartConditions.DependsOn[%d] %q: must be lowercase [a-z0-9._-] starting alphanumeric", index, dependency)
		} else if dependency == cfg.AppNameID {
			add("StartConditions.DependsOn[%d]: an app cannot depend on itself", index)
		}
	}
	if start.Attached && !start.Terminal {
		add("StartConditions.Attached: requires Terminal")
	}
	if cfg.Type == schema.ZincContainer && start.Terminal && cfg.StopConditions.Background && !start.Attached {
		add("StartConditions.Terminal: cannot also be StopConditions.Background without Attached")
	}
	if start.Attached && strings.TrimSpace(start.Entrypoint) == "" && strings.TrimSpace(start.AttachedEntrypoint) == "" {
		add("StartConditions.Attached: needs an explicit Entrypoint or AttachedEntrypoint")
	}
	if !start.Attached && (start.AttachedEntrypoint != "" || len(start.AttachedEnv) > 0) {
		add("StartConditions.AttachedEntrypoint/AttachedEnv: require Attached")
	}
	for _, entry := range []struct{ field, value string }{
		{"Entrypoint", start.Entrypoint}, {"AttachedEntrypoint", start.AttachedEntrypoint},
	} {
		if strings.ContainsRune(entry.value, '\x00') {
			add("StartConditions.%s: must not contain NUL", entry.field)
		}
	}
	if start.LoaderBIOS && start.SecureBoot {
		add("StartConditions.SecureBoot: requires UEFI (LoaderBIOS must be false)")
	}
}

func checkResources(resources schema.ResourcesMeta, add addFunc) {
	if math.IsNaN(resources.MaxCPUCores) || math.IsInf(resources.MaxCPUCores, 0) {
		add("ResourcesMeta.MaxCPUCores %v: must be finite", resources.MaxCPUCores)
	} else if resources.MaxCPUCores < 0 {
		add("ResourcesMeta.MaxCPUCores %v: must be >= 0 (0 = unlimited)", resources.MaxCPUCores)
	}
	if resources.MaxRamMiB < 0 {
		add("ResourcesMeta.MaxRamMiB %d: must be >= 0 (0 = unlimited)", resources.MaxRamMiB)
	}
	if resources.PIDsLimit < 0 {
		add("ResourcesMeta.PIDsLimit %d: must be >= 0 (0 = unlimited)", resources.PIDsLimit)
	}
}

// Raw backend options are argv, not shell input. The user deliberately opts out
// of the structured policy; Warnings surfaces that choice without an allowlist.
func checkRawFlags(field string, flags []string, add addFunc) {
	for index, argument := range flags {
		if strings.ContainsRune(argument, '\x00') {
			add("%s[%d]: argv strings must not contain NUL", field, index)
		}
	}
}

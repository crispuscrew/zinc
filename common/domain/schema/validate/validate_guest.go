package validate

import "github.com/crispuscrew/zinc/common/domain/schema"

// Host-side lifecycle, network, display and audio policy is shared. These fields
// need a guest-side bridge that the current runner cannot provide.
func checkGuestUnsupported(cfg schema.AppConfig, add addFunc) {
	start := cfg.StartConditions
	for _, field := range []struct {
		set          bool
		name, reason string
	}{
		{len(cfg.Keys) > 0, "Keys", "guest key mounts are unavailable; use ImageMeta.PublicSSHKeyPath for a public SSH key"},
		{len(cfg.Volumes) > 0, "Volumes", "guest filesystem sharing is unavailable"},
		{len(cfg.Configs) > 0, "Configs", "guest filesystem sharing is unavailable"},
		{cfg.HostTheme, "HostTheme", "guest theme sharing is unavailable"},
		{!cfg.DBusMeta.IsZero(), "DBusMeta", "the filtered host session bus needs a guest-side bridge"},
		{cfg.NotificationMeta != (schema.NotificationMeta{}), "NotificationMeta", "notification filtering needs a guest-side bus bridge"},
		{cfg.DisplayMeta.RequireSecurityContext, "DisplayMeta.RequireSecurityContext", "the guest-side compositor security-context bridge is unavailable"},
		{cfg.InternalUserMeta.KeepUserID, "InternalUserMeta.KeepUserID", "host UID mapping into the guest is unavailable"},
		{start.Entrypoint != "", "StartConditions.Entrypoint", "the guest boots its own init; guest command execution is unavailable"},
		{len(start.EntrypointEnv) > 0, "StartConditions.EntrypointEnv", "guest process environment injection is unavailable"},
		{start.Attached, "StartConditions.Attached", "guest attached entrypoint execution is unavailable"},
		{start.AttachedEntrypoint != "", "StartConditions.AttachedEntrypoint", "guest command execution is unavailable"},
		{len(start.AttachedEnv) > 0, "StartConditions.AttachedEnv", "guest process environment injection is unavailable"},
	} {
		if field.set {
			add("%s: not supported for a VM app (%s)", field.name, field.reason)
		}
	}
}

package validate

import (
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

const notifyBusName = "org.freedesktop.Notifications"

func checkNotifications(cfg schema.AppConfig, add addFunc) {
	notify := cfg.NotificationMeta
	if notify == (schema.NotificationMeta{}) {
		return
	}
	if cfg.DBusMeta.IsZero() {
		add("NotificationMeta: needs a session bus; add %q to DBusMeta.Talk, or leave this block at its defaults", notifyBusName)
	} else if !talksTo(cfg.DBusMeta.Talk, notifyBusName) {
		add("NotificationMeta: this app is not allowed to reach %s; add that name to DBusMeta.Talk", notifyBusName)
	}
	if notify.Disabled && notify.Silenced {
		add("NotificationMeta: Disabled and Silenced are opposites; set at most one")
	}
	if notify.UseCustomPrefix && strings.TrimSpace(notify.CustomPrefix) == "" {
		add("NotificationMeta.CustomPrefix: required when UseCustomPrefix is set")
	}
	if !notify.UseCustomPrefix && notify.CustomPrefix != "" {
		add("NotificationMeta.CustomPrefix %q: set UseCustomPrefix to apply it, or clear it", notify.CustomPrefix)
	}
	if hasControl(notify.CustomPrefix) {
		add("NotificationMeta.CustomPrefix: must be a single line with no control characters")
	}
}

func talksTo(talk []string, name string) bool {
	for _, entry := range talk {
		if entry == name {
			return true
		}
		if base, found := strings.CutSuffix(entry, ".*"); found && strings.HasPrefix(name, base+".") {
			return true
		}
	}
	return false
}

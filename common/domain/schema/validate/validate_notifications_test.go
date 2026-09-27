package validate

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestNotificationBusAndPrefixPolicy(testing *testing.T) {
	cfg := baseCfg()
	if err := Validate(cfg); err != nil {
		testing.Fatal(err)
	}
	cfg.NotificationMeta.Silenced = true
	requireError(testing, cfg, "needs a session bus")
	cfg.InternalUserMeta.KeepUserID = true
	cfg.DBusMeta.Talk = []string{"org.freedesktop.portal.Desktop"}
	requireError(testing, cfg, "not allowed to reach")
	cfg.DBusMeta.Talk = []string{notifyBusName}
	if err := Validate(cfg); err != nil {
		testing.Fatal(err)
	}
	cfg.NotificationMeta.Disabled = true
	requireError(testing, cfg, "opposites")
	cfg.NotificationMeta = schema.NotificationMeta{UseCustomPrefix: true}
	requireError(testing, cfg, "CustomPrefix: required")
	cfg.NotificationMeta = schema.NotificationMeta{CustomPrefix: "[work]"}
	requireError(testing, cfg, "set UseCustomPrefix")
	cfg.NotificationMeta.UseCustomPrefix = true
	if err := Validate(cfg); err != nil {
		testing.Fatal(err)
	}
	cfg.NotificationMeta.CustomPrefix = "[work]\nSystem"
	requireError(testing, cfg, "single line")
}

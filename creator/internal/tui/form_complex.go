package tui

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func (frm *formModel) complexFields() []formField {
	cfg := frm.draft
	return []formField{
		{label: "network", kind: kindInfo, info: func() string {
			return fmt.Sprintf("NICs=%d ordered rules=%d DNS=%d; edit full policy in advanced YAML", len(frm.draft.NetworkMeta.Interfaces), len(frm.draft.NetworkMeta.RulesByPriority), len(frm.draft.NetworkMeta.DNS.ResolversByPriority))
		}},
		{label: "files", kind: kindInfo, info: func() string {
			return fmt.Sprintf("volumes=%d configs=%d keys=%d; edit in advanced YAML", len(frm.draft.Volumes), len(frm.draft.Configs), len(frm.draft.Keys))
		}},
		frm.text("dbus.talk", &frm.dbusTalk, strings.Join(cfg.DBusMeta.Talk, ", "), func(cfg *schema.AppConfig, value string) error { cfg.DBusMeta.Talk = splitCommas(value); return nil }),
		frm.text("dbus.own", &frm.dbusOwn, strings.Join(cfg.DBusMeta.Own, ", "), func(cfg *schema.AppConfig, value string) error { cfg.DBusMeta.Own = splitCommas(value); return nil }),
		frm.structured("notifications (YAML)", cfg.NotificationMeta, func(cfg *schema.AppConfig, value string) error {
			var notify schema.NotificationMeta
			err := parseStructured(value, &notify)
			cfg.NotificationMeta = notify
			return err
		}),
		frm.structured("creator flags (argv array)", cfg.CreatorFlags, func(cfg *schema.AppConfig, value string) error {
			var args []string
			err := parseStructured(value, &args)
			cfg.CreatorFlags = args
			return err
		}),
		frm.structured("runner flags (argv array)", cfg.RunnerFlags, func(cfg *schema.AppConfig, value string) error {
			var args []string
			err := parseStructured(value, &args)
			cfg.RunnerFlags = args
			return err
		}),
	}
}

func (frm *formModel) advancedSummary() string {
	return fmt.Sprintf("schema=%d inherits=%q; Launcher/Start/Stop/Resources/User/Image/Display/Network/Notifications/DBus/Configs/Volumes/Keys/Theme/Audio/Flags: edit all YAML",
		frm.draft.SchemaVersion, frm.draft.Inherits)
}

func (frm *formModel) rawFlagsPresent() bool {
	for _, binding := range frm.bindings {
		if binding.label != "creator flags (argv array)" && binding.label != "runner flags (argv array)" {
			continue
		}
		var args []string
		if parseStructured(binding.value(), &args) != nil || len(args) > 0 {
			return true
		}
	}
	return false
}

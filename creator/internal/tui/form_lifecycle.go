package tui

import (
	"github.com/crispuscrew/zinc/common/domain/schema"
	"strings"
)

func (frm *formModel) lifecycleFields() []formField {
	start := frm.draft.StartConditions
	return []formField{
		frm.text("entrypoint", &frm.entrypoint, start.Entrypoint, func(cfg *schema.AppConfig, value string) error {
			cfg.StartConditions.Entrypoint = strings.TrimSpace(value)
			return nil
		}),
		frm.structured("entrypoint env (YAML map)", start.EntrypointEnv, func(cfg *schema.AppConfig, value string) error {
			var env map[string]string
			err := parseStructured(value, &env)
			cfg.StartConditions.EntrypointEnv = env
			return err
		}),
		toggle("terminal", func() bool { return frm.draft.StartConditions.Terminal }, func(value bool) { frm.draft.StartConditions.Terminal = value }),
		toggle("attached", func() bool { return frm.draft.StartConditions.Attached }, func(value bool) { frm.draft.StartConditions.Attached = value }),
		frm.scalar("attached entrypoint", start.AttachedEntrypoint, func(cfg *schema.AppConfig, value string) error {
			cfg.StartConditions.AttachedEntrypoint = strings.TrimSpace(value)
			return nil
		}),
		frm.structured("attached env (YAML map)", start.AttachedEnv, func(cfg *schema.AppConfig, value string) error {
			var env map[string]string
			err := parseStructured(value, &env)
			cfg.StartConditions.AttachedEnv = env
			return err
		}),
		frm.structured("depends on (YAML list)", start.DependsOn, func(cfg *schema.AppConfig, value string) error {
			var names []string
			err := parseStructured(value, &names)
			cfg.StartConditions.DependsOn = names
			return err
		}),
		toggle("read_only_rootfs", func() bool { return frm.draft.StartConditions.ReadOnlyRootfs }, func(value bool) { frm.draft.StartConditions.ReadOnlyRootfs = value }),
		toggle("autorestart", func() bool { return frm.draft.StopConditions.Autorestart }, func(value bool) { frm.draft.StopConditions.Autorestart = value }),
		toggle("keep_alive", func() bool { return frm.draft.StopConditions.KeepAlive }, func(value bool) { frm.draft.StopConditions.KeepAlive = value }),
		toggle("background", func() bool { return frm.draft.StopConditions.Background }, func(value bool) { frm.draft.StopConditions.Background = value }),
	}
}

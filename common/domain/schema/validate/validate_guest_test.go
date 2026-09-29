package validate

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestVMGuestFeaturesFailExplicitly(test *testing.T) {
	for _, testCase := range []struct {
		field  string
		change func(*schema.AppConfig)
	}{
		{"Keys", func(cfg *schema.AppConfig) { cfg.Keys = []schema.Key{{Type: schema.SSH, Path: "/home/user/key"}} }},
		{"Volumes", func(cfg *schema.AppConfig) { cfg.Volumes = []schema.Volume{{InnerMount: "/data"}} }},
		{"Configs", func(cfg *schema.AppConfig) {
			cfg.Configs = []schema.ConfigFile{{BundlePath: "app.conf", InnerMount: "/app.conf"}}
		}},
		{"HostTheme", func(cfg *schema.AppConfig) { cfg.HostTheme = true }},
		{"DBusMeta", func(cfg *schema.AppConfig) { cfg.DBusMeta.Talk = []string{notifyBusName} }},
		{"NotificationMeta", func(cfg *schema.AppConfig) { cfg.NotificationMeta.Silenced = true }},
		{"RequireSecurityContext", func(cfg *schema.AppConfig) { cfg.DisplayMeta.RequireSecurityContext = true }},
		{"KeepUserID", func(cfg *schema.AppConfig) { cfg.InternalUserMeta.KeepUserID = true }},
		{"Entrypoint", func(cfg *schema.AppConfig) { cfg.StartConditions.Entrypoint = "sh" }},
		{"EntrypointEnv", func(cfg *schema.AppConfig) { cfg.StartConditions.EntrypointEnv = map[string]string{"LANG": "C"} }},
		{"Attached", func(cfg *schema.AppConfig) { cfg.StartConditions.Attached = true }},
		{"AttachedEntrypoint", func(cfg *schema.AppConfig) { cfg.StartConditions.AttachedEntrypoint = "sh" }},
		{"AttachedEnv", func(cfg *schema.AppConfig) { cfg.StartConditions.AttachedEnv = map[string]string{"LANG": "C"} }},
	} {
		test.Run(testCase.field, func(test *testing.T) {
			cfg := baseVM()
			testCase.change(&cfg)
			requireError(test, cfg, testCase.field)
			requireError(test, cfg, "not supported for a VM app")
		})
	}
}

func TestVMReadOnlyRootfsIsHostEnforced(test *testing.T) {
	cfg := baseVM()
	cfg.StartConditions.ReadOnlyRootfs = true
	if err := Validate(cfg); err != nil {
		test.Fatal(err)
	}
}

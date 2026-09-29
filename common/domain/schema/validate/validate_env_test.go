package validate

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestMovedEnvironments(testing *testing.T) {
	for _, field := range []string{"EntrypointEnv", "AttachedEnv"} {
		build := func(environment map[string]string) schema.AppConfig {
			cfg := baseCfg()
			if field == "EntrypointEnv" {
				cfg.StartConditions.EntrypointEnv = environment
			} else {
				cfg.StartConditions = schema.StartConditions{Terminal: true, Attached: true, AttachedEntrypoint: "sh", AttachedEnv: environment}
			}
			return cfg
		}
		for _, name := range []string{"BAD NAME", "BAD=NAME", "1LEADING", "has-dash", "", "XDG_RUNTIME_DIR", "WAYLAND_DISPLAY", "DBUS_SESSION_BUS_ADDRESS"} {
			requireError(testing, build(map[string]string{name: "value"}), field)
		}
		for _, value := range []string{"two\nlines", "bell\x07", "nul\x00"} {
			requireError(testing, build(map[string]string{"VAR": value}), field)
		}
		if err := Validate(build(map[string]string{"LANG": "en_US.UTF-8", "_UNDER": "", "A1": "two words"})); err != nil {
			testing.Fatal(err)
		}
	}
}

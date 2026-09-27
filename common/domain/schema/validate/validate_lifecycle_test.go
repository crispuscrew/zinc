package validate

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestAttachedLifecycle(testing *testing.T) {
	cfg := baseCfg()
	cfg.StartConditions.Attached = true
	requireError(testing, cfg, "requires Terminal")
	cfg.StartConditions.Terminal = true
	requireError(testing, cfg, "explicit Entrypoint")
	cfg.StartConditions.AttachedEntrypoint = "sh"
	cfg.StartConditions.AttachedEnv = map[string]string{"LANG": "C"}
	cfg.StopConditions = schema.StopConditions{KeepAlive: true, Background: true, Autorestart: true}
	if err := Validate(cfg); err != nil {
		testing.Fatal(err)
	}
	cfg.StartConditions.Attached = false
	requireError(testing, cfg, "AttachedEntrypoint/AttachedEnv")
	cfg.StartConditions = schema.StartConditions{Terminal: true}
	requireError(testing, cfg, "Background")
	cfg.StopConditions.Background = false
	if err := Validate(cfg); err != nil {
		testing.Fatal(err)
	}
}

func TestRawBackendArgvIsExplicitOptOut(testing *testing.T) {
	for _, base := range []func() schema.AppConfig{baseCfg, baseVM} {
		cfg := base()
		cfg.CreatorFlags = []string{"--privileged", "", "argument with spaces", "two\nlines"}
		cfg.RunnerFlags = []string{"--network=host", "-device", "anything,property=value"}
		if err := Validate(cfg); err != nil {
			testing.Fatalf("raw argv must not have an allowlist: %v", err)
		}
		warnings := strings.Join(Warnings(cfg), "\n")
		for _, field := range []string{"CreatorFlags", "RunnerFlags", "unsafe raw backend"} {
			if !strings.Contains(warnings, field) {
				testing.Errorf("missing %s warning: %s", field, warnings)
			}
		}
		cfg.CreatorFlags = []string{"bad\x00arg"}
		cfg.RunnerFlags = []string{"\x00"}
		requireError(testing, cfg, "CreatorFlags[0]")
		requireError(testing, cfg, "RunnerFlags[0]")
	}
}

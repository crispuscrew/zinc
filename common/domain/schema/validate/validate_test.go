package validate

import (
	"math"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func baseCfg() schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer,
		AppNameID: "app", ImageMeta: schema.ImageMeta{Image: "localhost/app:local"},
	}
}

func requireError(test *testing.T, cfg schema.AppConfig, field string) {
	test.Helper()
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), field) {
		test.Fatalf("want error containing %q, got %v", field, err)
	}
}

func TestSharedValidation(test *testing.T) {
	for _, testCase := range []struct {
		field  string
		change func(*schema.AppConfig)
	}{
		{"SchemaVersion", func(cfg *schema.AppConfig) { cfg.SchemaVersion = 0 }},
		{"Type", func(cfg *schema.AppConfig) { cfg.Type = "ZincJail" }},
		{"AppNameID", func(cfg *schema.AppConfig) { cfg.AppNameID = "../../escape" }},
		{"Inherits", func(cfg *schema.AppConfig) { cfg.Inherits = "../base" }},
		{"Inherits", func(cfg *schema.AppConfig) { cfg.Inherits = cfg.AppNameID }},
		{"DependsOn", func(cfg *schema.AppConfig) { cfg.StartConditions.DependsOn = []string{"../other"} }},
		{"DependsOn", func(cfg *schema.AppConfig) { cfg.StartConditions.DependsOn = []string{cfg.AppNameID} }},
		{"control characters", func(cfg *schema.AppConfig) { cfg.ImageMeta.Install = []string{"true\nFROM attacker"} }},
		{"finite", func(cfg *schema.AppConfig) { cfg.ResourcesMeta.MaxCPUCores = math.Inf(1) }},
		{"finite", func(cfg *schema.AppConfig) { cfg.ResourcesMeta.MaxCPUCores = math.NaN() }},
		{"MaxCPUCores", func(cfg *schema.AppConfig) { cfg.ResourcesMeta.MaxCPUCores = -1 }},
		{"MaxRamMiB", func(cfg *schema.AppConfig) { cfg.ResourcesMeta.MaxRamMiB = -1 }},
		{"PIDsLimit", func(cfg *schema.AppConfig) { cfg.ResourcesMeta.PIDsLimit = -1 }},
		{"NonRootUserName", func(cfg *schema.AppConfig) { cfg.InternalUserMeta.UseNonRootUser = true }},
		{"no effect", func(cfg *schema.AppConfig) { cfg.InternalUserMeta.NonRootUserName = "app" }},
		{"NonRootUserName", func(cfg *schema.AppConfig) { cfg.InternalUserMeta.NonRootUserName = "bad name" }},
	} {
		test.Run(testCase.field, func(test *testing.T) {
			cfg := baseCfg()
			testCase.change(&cfg)
			requireError(test, cfg, testCase.field)
		})
	}
	cfg := baseCfg()
	cfg.ImageMeta.Install = []string{"apk add --no-cache firefox", "adduser -D app"}
	cfg.StartConditions.DependsOn = []string{"vpn", "db-1"}
	cfg.InternalUserMeta = schema.InternalUserMeta{UseNonRootUser: true, NonRootUserName: "app", KeepUserID: true}
	cfg.ResourcesMeta.MaxCPUCores = 0.5
	if err := Validate(cfg); err != nil {
		test.Fatal(err)
	}
}

func TestContainerImageSecurity(test *testing.T) {
	const digest = "@sha256:1111111111111111111111111111111111111111111111111111111111111111"
	for _, image := range []string{"docker.io/library/alpine:3.20", "-v/:/host" + digest, "--privileged" + digest, "localhost/a\nFROM evil"} {
		cfg := baseCfg()
		cfg.ImageMeta.Image = image
		requireError(test, cfg, "ImageMeta.Image")
	}
	cfg := baseCfg()
	cfg.ImageMeta.Image = "registry.example:5000/library/alpine" + digest
	if err := Validate(cfg); err != nil {
		test.Fatal(err)
	}
}

func TestErrorsAreJoined(test *testing.T) {
	cfg := baseCfg()
	cfg.AppNameID = "../bad"
	cfg.ResourcesMeta.MaxRamMiB = -1
	cfg.RunnerFlags = []string{"bad\x00argument"}
	for _, field := range []string{"AppNameID", "MaxRamMiB", "RunnerFlags"} {
		requireError(test, cfg, field)
	}
}

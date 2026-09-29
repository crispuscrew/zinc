package validate

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// The two flags are opposites. Accepting both would mean one of them silently doing nothing,
// which is the shape this schema refuses everywhere else.
func TestDisplay_TheTwoSecurityContextFlagsCannotBothBeSet(t *testing.T) {
	cfg := baseCfg()
	cfg.DisplayMeta.DisableSecurityContext = true
	cfg.DisplayMeta.RequireSecurityContext = true
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "opposites") {
		t.Fatalf("want a refusal for the contradictory pair, got: %v", err)
	}
}

func TestDisplay_EitherFlagAloneIsFine(t *testing.T) {
	for _, apply := range []func(*schema.AppConfig){
		func(cfg *schema.AppConfig) { cfg.DisplayMeta.DisableSecurityContext = true },
		func(cfg *schema.AppConfig) { cfg.DisplayMeta.RequireSecurityContext = true },
		func(cfg *schema.AppConfig) {},
	} {
		cfg := baseCfg()
		apply(&cfg)
		if err := Validate(cfg); err != nil {
			t.Errorf("DisplayMeta %+v was rejected: %v", cfg.DisplayMeta, err)
		}
	}
}

// A guest draws into a qemu window and never speaks the host compositor's protocol, so there
// is no security context to require. Refuse rather than accept a field that cannot apply.
func TestDisplay_RequireRefusedOnAVMApp(t *testing.T) {
	cfg := baseVM()
	cfg.DisplayMeta.RequireSecurityContext = true
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "RequireSecurityContext") {
		t.Fatalf("want a refusal on a VM app, got: %v", err)
	}
}

func TestSharedDisplayDimensions(test *testing.T) {
	for _, base := range []func() schema.AppConfig{baseCfg, baseVM} {
		for _, size := range [][2]int{{1920, 0}, {0, 1080}, {-1, 1080}, {1920, -1}} {
			cfg := base()
			cfg.DisplayMeta.DisplayWidth, cfg.DisplayMeta.DisplayHeight = size[0], size[1]
			requireError(test, cfg, "DisplayMeta.Display")
		}
		for _, size := range [][2]int{{0, 0}, {640, 480}, {1920, 1080}, {3840, 2160}} {
			cfg := base()
			cfg.DisplayMeta.DisplayWidth, cfg.DisplayMeta.DisplayHeight = size[0], size[1]
			if err := Validate(cfg); err != nil {
				test.Fatal(err)
			}
		}
		cfg := base()
		cfg.DisplayMeta = schema.DisplayMeta{DisableGpuAccess: true, Vulkan: true}
		requireError(test, cfg, "Vulkan")
	}
}

func TestGuestDisplayHardwareLimits(test *testing.T) {
	for _, size := range [][2]int{{320, 200}, {1921, 1080}, {4096, 2160}} {
		cfg := baseVM()
		cfg.DisplayMeta.DisplayWidth, cfg.DisplayMeta.DisplayHeight = size[0], size[1]
		requireError(test, cfg, "DisplayMeta.Display")
		cfg = baseCfg()
		cfg.DisplayMeta.DisplayWidth, cfg.DisplayMeta.DisplayHeight = size[0], size[1]
		if err := Validate(cfg); err != nil {
			test.Fatalf("guest hardware limits must not constrain a container display: %v", err)
		}
	}
	cfg := baseVM()
	cfg.StartConditions.LoaderBIOS = true
	cfg.DisplayMeta = schema.DisplayMeta{DisplayWidth: 1920, DisplayHeight: 1080}
	requireError(test, cfg, "fixed guest mode requires UEFI")
}

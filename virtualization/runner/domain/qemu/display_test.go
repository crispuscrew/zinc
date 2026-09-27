package qemu

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func TestDisplayInference(t *testing.T) {
	cfg, layout := fixture()
	if ResolveDisplay(cfg, layout.Runtime) != vmoptions.DisplayAccelerated {
		t.Fatal("default display")
	}
	cfg.DisplayMeta.DisableGpuAccess = true
	if ResolveDisplay(cfg, layout.Runtime) != vmoptions.DisplayCompatible {
		t.Fatal("GPU denial did not select software display")
	}
	cfg.StartConditions.Terminal = true
	if ResolveDisplay(cfg, layout.Runtime) != vmoptions.DisplayNone {
		t.Fatal("terminal did not select serial display")
	}
	layout.Runtime.Display = vmoptions.DisplayWindow
	if ResolveDisplay(cfg, layout.Runtime) != vmoptions.DisplayWindow {
		t.Fatal("explicit runtime display ignored")
	}
	layout.Runtime.Display = vmoptions.DisplayAccelerated
	if Validate(cfg, layout) == nil {
		t.Fatal("GPU conflict accepted")
	}
}

func TestVulkanRequiresAccelerationAndWarns(t *testing.T) {
	cfg, layout := fixture()
	cfg.DisplayMeta.Vulkan = true
	args := Args(cfg, layout)
	if slices.Contains(args, "-sandbox") {
		t.Fatal("Venus cannot spawn with this sandbox")
	}
	if !strings.Contains(strings.Join(args, " "), "venus=on,blob=on,hostmem=8G") {
		t.Fatal(args)
	}
	if len(Warnings(cfg, layout.Runtime, false)) == 0 {
		t.Fatal("sandbox downgrade must warn")
	}
	layout.Runtime.Display = vmoptions.DisplayWindow
	if Validate(cfg, layout) == nil {
		t.Fatal("Vulkan on software display accepted")
	}
}

func TestUEFIAndFixedResolution(t *testing.T) {
	cfg, layout := fixture()
	layout.Runtime.Display = vmoptions.DisplayCompatible
	cfg.DisplayMeta.DisplayWidth, cfg.DisplayMeta.DisplayHeight = 3840, 2160
	if err := Validate(cfg, layout); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(Args(cfg, layout), " ")
	if !strings.Contains(text, "bochs-display,xres=3840,yres=2160") {
		t.Fatal(text)
	}
	cfg.StartConditions.LoaderBIOS = true
	if Validate(cfg, layout) == nil {
		t.Fatal("fixed-resolution BIOS accepted")
	}
	cfg.DisplayMeta.DisplayWidth, cfg.DisplayMeta.DisplayHeight = 0, 0
	cfg.StartConditions.SecureBoot = true
	if Validate(cfg, layout) == nil {
		t.Fatal("Secure Boot BIOS accepted")
	}
	cfg.StartConditions.LoaderBIOS = false
	if got := option(Args(cfg, layout), "-machine"); !strings.Contains(got, "smm=on") {
		t.Fatal(got)
	}
}

func TestMinimizeFingerprintPreservesExplicitIdentity(t *testing.T) {
	explicit := "06:11:22:33:44:55"
	if macFor("app", explicit, true) != explicit {
		t.Fatal("explicit MAC overwritten")
	}
	if !strings.HasPrefix(macFor("app", "", true), "02:") {
		t.Fatal("not locally administered")
	}
	if macFor("app", "", true) != macFor("app", "", true) {
		t.Fatal("unstable MAC")
	}
	cfg, layout := fixture()
	before := option(Args(cfg, layout), "-uuid")
	cfg.MinimizeFingerprint = true
	args := Args(cfg, layout)
	if option(args, "-uuid") != before {
		t.Fatal("fingerprint option rotated machine identity")
	}
	if !strings.Contains(option(args, "-smbios"), "manufacturer=Generic") || !slices.Contains(args, "-sandbox") {
		t.Fatal(args)
	}
}

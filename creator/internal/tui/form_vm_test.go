package tui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func labels(frm *formModel) []string {
	var result []string
	for _, field := range frm.fields {
		result = append(result, field.label)
	}
	return result
}
func hasLabel(frm *formModel, want string) bool { return slices.Contains(labels(frm), want) }

func TestForm_TypeSwitchPreservesSharedData(t *testing.T) {
	base := sample("app")
	base.StartConditions = schema.StartConditions{DependsOn: []string{"base"}, AttachedEntrypoint: "sh", EntrypointEnv: map[string]string{"TOKEN": "value"}}
	base.RunnerFlags = []string{"--label", "a b"}
	base.AudioMeta.Playback.PipeWireDevices = []string{"exact sink name"}
	base.DisplayMeta.DisplayWidth, base.DisplayMeta.DisplayHeight = 1920, 1080
	base.HostTheme = true
	frm := newForm(base, false)
	frm.draft.Type = schema.ZincVirtualization
	frm.buildFields()
	if !hasLabel(frm, "base digest") {
		t.Fatal("VM controls missing")
	}
	actual := frm.toConfig()
	base.Type = schema.ZincVirtualization
	if !reflect.DeepEqual(actual, base) {
		t.Fatalf("switching type destroyed shared fields:\n%+v\n%+v", actual, base)
	}
}

func TestForm_VMDraftValidatesBothDocuments(t *testing.T) {
	frm := newForm(schema.AppConfig{Type: schema.ZincVirtualization}, true)
	frm.name.SetValue("guest")
	frm.image.SetValue("/var/lib/zinc/images/fedora.qcow2")
	frm.baseDigest.SetValue("sha256:" + strings.Repeat("a", 64))
	frm.memory.SetValue("8192")
	frm.vcpus.SetValue("4")
	frm.diskSize.SetValue("40")
	frm.vm.Display = vmoptions.DisplayAccelerated
	cfg := frm.toConfig()
	if frm.err != nil {
		t.Fatal(frm.err)
	}
	if err := validate.Validate(cfg); err != nil {
		t.Fatal(err)
	}
	options, err := frm.options(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ResourcesMeta.MaxRamMiB != 8192 || cfg.ResourcesMeta.MaxCPUCores != 4 || options.DiskSizeGiB != 40 {
		t.Fatalf("sizing lost: %+v %+v", cfg.ResourcesMeta, options)
	}
}

func TestForm_VMNoOpPreservesAutomaticDisplayAndDevices(t *testing.T) {
	cfg := sample("guest")
	cfg.Type, cfg.ImageMeta.Image = schema.ZincVirtualization, "/images/base.qcow2"
	cfg.ResourcesMeta = schema.ResourcesMeta{MaxCPUCores: 2, MaxRamMiB: 4096}
	options := vmoptions.Default(cfg.AppNameID, cfg.ImageMeta.Image)
	options.BaseDigest = "sha256:" + strings.Repeat("a", 64)
	options.InstallMedia = []string{"/iso/guest tools.iso"}
	frm := newForm(cfg, false)
	frm.loadVM(options)
	actual, err := frm.options(frm.toConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*actual, options) {
		t.Fatalf("no-op changed runtime selections: %+v", actual)
	}
}

func TestForm_InvalidNumbersAreNotSilentlyUnlimited(t *testing.T) {
	for _, value := range []string{"nonsense", "-1", "NaN", "+Inf"} {
		frm := newForm(sample("app"), false)
		frm.vcpus.SetValue(value)
		frm.toConfig()
		if frm.err == nil {
			t.Errorf("CPU value %q was silently accepted", value)
		}
	}
}

func TestNextValue(t *testing.T) {
	values := []string{"A", "B", "C"}
	if nextValue(values, "A") != "B" || nextValue(values, "C") != "A" {
		t.Fatal("enum did not cycle")
	}
}

func TestFormDisplayEnumOffersEveryRuntimeMode(t *testing.T) {
	frm := newForm(schema.AppConfig{Type: schema.ZincVirtualization}, true)
	field := frm.fields[fieldIdx(frm, "display")]
	for _, mode := range []string{"", "None", "Window", "Accelerated", "Compatible"} {
		if !slices.Contains(field.values, mode) {
			t.Errorf("display mode %q missing", mode)
		}
	}
}

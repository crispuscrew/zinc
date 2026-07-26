package tui

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
)

// labels lists the form's current rows, which is how the type switch is observed.
func labels(frm *formModel) []string {
	out := make([]string, len(frm.fields))
	for index, field := range frm.fields {
		out[index] = field.label
	}
	return out
}

func hasLabel(frm *formModel, want string) bool {
	for _, label := range labels(frm) {
		if label == want {
			return true
		}
	}
	return false
}

// The type row decides the rest of the form. Offering a container's multiterminal toggle
// on a guest would be offering a setting the runtime refuses to honour.
func TestForm_TypeSwitchRebuildsTheFields(t *testing.T) {
	frm := newForm(schema.AppConfig{}, true)
	if !hasLabel(frm, "multiterminal") {
		t.Fatalf("a container form should offer container fields, got %v", labels(frm))
	}
	if hasLabel(frm, "vcpus") {
		t.Fatalf("a container form should not offer guest hardware, got %v", labels(frm))
	}

	frm.draft.Type = schema.ZincVirtualization
	frm.buildFields()

	for _, want := range []string{"base image", "base digest", "memory (MiB)", "vcpus", "display", "cloud-init user"} {
		if !hasLabel(frm, want) {
			t.Errorf("a VM form should offer %q, got %v", want, labels(frm))
		}
	}
	for _, unwanted := range []string{"multiterminal", "terminal", "entrypoint", "host_theme"} {
		if hasLabel(frm, unwanted) {
			t.Errorf("a VM form must not offer %q: the runtime cannot honour it", unwanted)
		}
	}
}

// A form filled in as a VM must produce a config the shared validation accepts, or the
// author is told the app is invalid without ever having been offered the missing field.
func TestForm_VMDraftValidates(t *testing.T) {
	frm := newForm(schema.AppConfig{}, true)
	frm.draft.Type = schema.ZincVirtualization
	frm.buildFields()

	frm.name.SetValue("guest")
	frm.image.SetValue("/var/lib/zinc/images/fedora.qcow2")
	frm.baseDigest.SetValue("sha256:" + strings.Repeat("a", 64))
	frm.memory.SetValue("8192")
	frm.vcpus.SetValue("4")
	frm.diskSize.SetValue("40")
	frm.draft.VirtualizationMeta.Display = schema.VMDisplayAccelerated

	cfg := frm.toConfig()
	if err := validate.Validate(cfg); err != nil {
		t.Fatalf("the form's VM draft should validate, got: %v", err)
	}
	if cfg.VirtualizationMeta.MemoryMiB != 8192 || cfg.VirtualizationMeta.VCPUs != 4 {
		t.Errorf("sizing = %+v, want the typed values", cfg.VirtualizationMeta)
	}
}

// Switching an existing container app to a VM must clear the container-only fields it
// carried. Validation rejects them on a guest, so leaving them would block the save with
// settings the author never chose in this form and cannot see to remove.
func TestForm_SwitchingToVMClearsContainerOnlyFields(t *testing.T) {
	existing := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     "app",
		ImageMeta:     schema.ImageMeta{Image: "localhost/app:local"},
		Capabilities:  []string{"CAP_NET_BIND_SERVICE"},
		HostTheme:     true,
		Keys:          []schema.Key{{Type: schema.SSH, Path: "/home/u/.ssh/id_ed25519"}},
	}
	existing.StartConditions.Terminal = true
	existing.NetworkMeta.NetworkLists = []schema.NetworkList{{Host: true}}

	frm := newForm(existing, false)
	frm.draft.Type = schema.ZincVirtualization
	frm.buildFields()
	frm.image.SetValue("/var/lib/zinc/images/fedora.qcow2")
	frm.baseDigest.SetValue("sha256:" + strings.Repeat("b", 64))
	frm.memory.SetValue("4096")
	frm.vcpus.SetValue("2")
	frm.draft.VirtualizationMeta.Display = schema.VMDisplayNone

	cfg := frm.toConfig()
	if err := validate.Validate(cfg); err != nil {
		t.Fatalf("converting a container app to a VM should produce a valid config, got: %v", err)
	}
	if len(cfg.Capabilities) != 0 || len(cfg.NetworkMeta.NetworkLists) != 0 || len(cfg.Keys) != 0 || cfg.HostTheme {
		t.Errorf("container-only fields survived the switch: %+v", cfg)
	}
}

// The mirror: a container draft must carry no VM fields, which validation also rejects.
func TestForm_ContainerDraftCarriesNoVMFields(t *testing.T) {
	frm := newForm(schema.AppConfig{}, true)
	frm.name.SetValue("app")
	frm.image.SetValue("localhost/app:local")
	frm.memory.SetValue("8192") // typed while the form was briefly a VM

	cfg := frm.toConfig()
	if !cfg.VirtualizationMeta.IsZero() {
		t.Errorf("a container draft should carry no VM fields, got %+v", cfg.VirtualizationMeta)
	}
	if err := validate.Validate(cfg); err != nil {
		t.Fatalf("the container draft should validate, got: %v", err)
	}
}

// The enum cycles and wraps, and an unrecognised value from a hand-edited config lands on
// the first rather than sticking.
func TestNextValue(t *testing.T) {
	values := []string{"A", "B", "C"}
	if got := nextValue(values, "A"); got != "B" {
		t.Errorf("nextValue(A) = %q, want B", got)
	}
	if got := nextValue(values, "C"); got != "A" {
		t.Errorf("nextValue(C) should wrap to A, got %q", got)
	}
	if got := nextValue(values, "nonsense"); got != "A" {
		t.Errorf("nextValue(unknown) = %q, want the first value", got)
	}
}

// An unreadable number is left unset so validation names the field, rather than the form
// inventing a value the author did not type.
func TestParseNum(t *testing.T) {
	cases := map[string]int64{"8192": 8192, "": 0, "abc": 0, "-4": 0, " 2048 ": 2048}
	for input, want := range cases {
		if got := parseNum(input); got != want {
			t.Errorf("parseNum(%q) = %d, want %d", input, got, want)
		}
	}
}

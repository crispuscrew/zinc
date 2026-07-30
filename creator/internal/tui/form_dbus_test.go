package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
)

// containerDraft is a form on a minimal valid container app, so each test below changes only
// the bus rows it is about.
func containerDraft(t *testing.T) *formModel {
	t.Helper()
	frm := newForm(schema.AppConfig{}, true)
	frm.name.SetValue("notes")
	frm.image.SetValue("localhost/notes:local")
	return frm
}

// The form must offer the rows at all - the whole point of this change is that DBusMeta stops
// being reachable only through the advanced YAML editor.
func TestForm_ContainerOffersBusRows(t *testing.T) {
	frm := containerDraft(t)
	for _, want := range []string{"dbus.talk", "dbus.own"} {
		if !hasLabel(frm, want) {
			t.Errorf("container form has no %q row: %v", want, labels(frm))
		}
	}
}

// Filling the rows must produce a config that actually validates. A form that collected the
// grants and left KeepUserID alone could only ever fail to save, since validation refuses that
// pair - so this test is really about the implication, not the parsing.
func TestForm_BusGrantsImplyKeepUserID(t *testing.T) {
	frm := containerDraft(t)
	frm.dbusTalk.SetValue("org.freedesktop.portal.Desktop, org.freedesktop.Notifications")
	frm.dbusOwn.SetValue("org.mpris.MediaPlayer2.notes")

	cfg := frm.toConfig()
	if err := validate.Validate(cfg); err != nil {
		t.Fatalf("a form with bus grants produced an unsavable config: %v", err)
	}
	if !cfg.InternalUserMeta.KeepUserID {
		t.Error("bus grants did not imply KeepUserID, so this config could never be saved")
	}
	if want := []string{"org.freedesktop.portal.Desktop", "org.freedesktop.Notifications"}; !slices.Equal(cfg.DBusMeta.Talk, want) {
		t.Errorf("Talk = %v, want %v (comma-separated, trimmed)", cfg.DBusMeta.Talk, want)
	}
	if want := []string{"org.mpris.MediaPlayer2.notes"}; !slices.Equal(cfg.DBusMeta.Own, want) {
		t.Errorf("Own = %v, want %v", cfg.DBusMeta.Own, want)
	}
}

// Empty rows must leave the app with no bus AND no KeepUserID. The default is the security
// property here, so it must not be a side effect of the rows merely existing.
func TestForm_EmptyBusRowsGrantNothing(t *testing.T) {
	cfg := containerDraft(t).toConfig()
	if !cfg.DBusMeta.IsZero() {
		t.Errorf("empty bus rows produced grants: %+v", cfg.DBusMeta)
	}
	if cfg.InternalUserMeta.KeepUserID {
		t.Error("empty bus rows set KeepUserID, changing who the app runs as for no reason")
	}
}

// A trailing comma is a typo, not a bus name.
func TestForm_TrailingCommaIsNotAName(t *testing.T) {
	frm := containerDraft(t)
	frm.dbusTalk.SetValue("org.freedesktop.portal.Desktop,")
	cfg := frm.toConfig()
	if len(cfg.DBusMeta.Talk) != 1 {
		t.Errorf("Talk = %v, want one entry", cfg.DBusMeta.Talk)
	}
	if err := validate.Validate(cfg); err != nil {
		t.Errorf("a trailing comma made the config invalid: %v", err)
	}
}

// Switching a bus-granted container app to a VM must clear the grants, like every other
// container-only field, or the save would fail on a setting the author never chose here.
func TestForm_SwitchingToVMClearsBusGrants(t *testing.T) {
	existing := schema.AppConfig{
		SchemaVersion:    schema.SchemaVersion,
		Type:             schema.ZincContainer,
		AppNameID:        "notes",
		ImageMeta:        schema.ImageMeta{Image: "localhost/notes:local"},
		InternalUserMeta: schema.InternalUserMeta{KeepUserID: true},
		DBusMeta:         schema.DBusMeta{Talk: []string{"org.freedesktop.portal.Desktop"}},
	}

	frm := newForm(existing, false)
	frm.draft.Type = schema.ZincVirtualization
	frm.buildFields()
	frm.image.SetValue("/var/lib/zinc/images/fedora.qcow2")
	frm.baseDigest.SetValue("sha256:" + strings.Repeat("b", 64))
	frm.memory.SetValue("4096")
	frm.vcpus.SetValue("2")
	frm.draft.VirtualizationMeta.Display = schema.VMDisplayNone

	cfg := frm.toConfig()
	if !cfg.DBusMeta.IsZero() {
		t.Errorf("bus grants survived the switch to a VM: %+v", cfg.DBusMeta)
	}
	if err := validate.Validate(cfg); err != nil {
		t.Fatalf("converting a bus-granted container app to a VM should validate, got: %v", err)
	}
}

// A VM form must not offer the rows at all.
func TestForm_VMOffersNoBusRows(t *testing.T) {
	frm := newForm(schema.AppConfig{Type: schema.ZincVirtualization}, true)
	for _, unwanted := range []string{"dbus.talk", "dbus.own"} {
		if hasLabel(frm, unwanted) {
			t.Errorf("VM form offers %q, which a guest cannot honour", unwanted)
		}
	}
}

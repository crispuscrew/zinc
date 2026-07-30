package compose

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// A bus grant is a deliberate narrowing of a sandbox, and compose cannot carry it. Exporting
// must therefore SAY so: an importer of the produced file gets an app with no bus at all, and
// the danger is not that outcome (it is the fail-closed default) but someone believing the
// file carried the decision.
func TestExport_BusGrantsAreReportedAsLost(t *testing.T) {
	cfg := schema.AppConfig{
		SchemaVersion:    schema.SchemaVersion,
		Type:             schema.ZincContainer,
		AppNameID:        "notes",
		ImageMeta:        schema.ImageMeta{Image: "localhost/notes:local"},
		InternalUserMeta: schema.InternalUserMeta{KeepUserID: true},
		DBusMeta: schema.DBusMeta{
			Talk: []string{"org.freedesktop.portal.Desktop"},
			Own:  []string{"org.mpris.MediaPlayer2.notes"},
		},
	}

	_, lost, err := FromApp(cfg)
	if err != nil {
		t.Fatalf("FromApp: %v", err)
	}
	joined := strings.Join(lost, "\n")
	if !strings.Contains(joined, "DBusMeta") {
		t.Errorf("exporting an app with bus grants did not report them as lost:\n%s", joined)
	}
	// The note has to state the consequence, not just the omission, or a reader takes silence
	// on the import side for "it round-trips".
	if !strings.Contains(joined, "NO session bus") {
		t.Errorf("the note does not say what an importer actually gets:\n%s", joined)
	}
}

// The mirror: an app with no grants must not produce a note about a feature it never used.
func TestExport_NoBusNoNote(t *testing.T) {
	cfg := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincContainer,
		AppNameID:     "plain",
		ImageMeta:     schema.ImageMeta{Image: "localhost/plain:local"},
	}
	_, lost, err := FromApp(cfg)
	if err != nil {
		t.Fatalf("FromApp: %v", err)
	}
	if joined := strings.Join(lost, "\n"); strings.Contains(joined, "DBusMeta") {
		t.Errorf("an app with no bus grants got a DBusMeta note:\n%s", joined)
	}
}

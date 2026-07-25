package picker

import "testing"

func sampleApps() []App {
	return []App{
		{Name: "alacritty", Description: "terminal"},
		{Name: "firefox", Description: "browser"},
		{Name: "syncthing", Description: "sync"},
	}
}

// Typing narrows the list to matching apps; clearing the query restores it.
func TestType_NarrowsAndRestores(t *testing.T) {
	mdl := New(sampleApps())
	if len(mdl.Visible()) != 3 {
		t.Fatalf("empty query should show all 3, got %d", len(mdl.Visible()))
	}
	mdl.Type("fire")
	visible := mdl.Visible()
	if len(visible) != 1 || visible[0].Name != "firefox" {
		t.Fatalf("query 'fire' should match only firefox, got %v", names(visible))
	}
	mdl.ClearQuery()
	if len(mdl.Visible()) != 3 {
		t.Fatalf("clearing the query should restore all 3, got %d", len(mdl.Visible()))
	}
}

// Backspace deletes one rune at a time and refilters; it is unicode-safe.
func TestBackspace_DeletesOneRune(t *testing.T) {
	mdl := New(sampleApps())
	mdl.Type("firé") // trailing multibyte rune: must not corrupt the query on delete
	mdl.Backspace()
	if mdl.Query() != "fir" {
		t.Fatalf("backspace over a multibyte rune should leave 'fir', got %q", mdl.Query())
	}
	if len(mdl.Visible()) != 1 || mdl.Visible()[0].Name != "firefox" {
		t.Fatalf("query 'fir' should match firefox, got %v", names(mdl.Visible()))
	}
	mdl.Backspace()
	mdl.Backspace()
	mdl.Backspace()
	if mdl.Query() != "" {
		t.Fatalf("backspacing to empty should clear the query, got %q", mdl.Query())
	}
	mdl.Backspace() // no-op on empty
	if mdl.Query() != "" {
		t.Fatalf("backspace on empty is a no-op, got %q", mdl.Query())
	}
}

// The cursor moves within the filtered range and clamps at both ends.
func TestMoveCursor_ClampsToRange(t *testing.T) {
	mdl := New(sampleApps()) // 3 apps, cursor 0
	mdl.MoveCursor(1)
	if mdl.Cursor() != 1 {
		t.Fatalf("down -> 1, got %d", mdl.Cursor())
	}
	mdl.MoveCursor(5) // past the end
	if mdl.Cursor() != 2 {
		t.Fatalf("past end clamps at 2, got %d", mdl.Cursor())
	}
	mdl.MoveCursor(-10) // past the start
	if mdl.Cursor() != 0 {
		t.Fatalf("past start clamps at 0, got %d", mdl.Cursor())
	}
}

// A new query resets the cursor to the top (the best match).
func TestRefilter_ResetsCursor(t *testing.T) {
	mdl := New(sampleApps())
	mdl.MoveCursor(2)
	mdl.Type("s") // matches syncthing (and alacritty via subsequence); cursor should reset
	if mdl.Cursor() != 0 {
		t.Fatalf("a new query should reset the cursor to 0, got %d", mdl.Cursor())
	}
}

// Selected returns the highlighted app, and reports ok=false when nothing matches.
func TestSelected(t *testing.T) {
	mdl := New(sampleApps())
	mdl.Type("fire")
	app, ok := mdl.Selected()
	if !ok || app.Name != "firefox" {
		t.Fatalf("selected should be firefox, got %q ok=%v", app.Name, ok)
	}
	mdl.Type("zzz") // now 'firezzz' matches nothing
	if len(mdl.Visible()) != 0 {
		t.Fatalf("precondition: no matches, got %d", len(mdl.Visible()))
	}
	if _, ok := mdl.Selected(); ok {
		t.Fatal("Selected must report ok=false when nothing matches")
	}
}

// SelectedIndex (the method the menu uses to map a pick back to the caller's items) returns
// the index into the ORIGINAL apps slice, tracking the filter and the cursor, and reports
// ok=false when nothing matches.
func TestSelectedIndex(t *testing.T) {
	apps := sampleApps() // caller order: alacritty, firefox, syncthing
	mdl := New(apps)
	mdl.Type("fire")
	index, ok := mdl.SelectedIndex()
	if !ok || apps[index].Name != "firefox" {
		t.Fatalf("cursor should select firefox at its original index, got index=%d ok=%v", index, ok)
	}
	mdl.ClearQuery()
	mdl.MoveCursor(2)
	index, ok = mdl.SelectedIndex()
	if !ok || apps[index].Name != "syncthing" {
		t.Fatalf("cursor 2 should map to syncthing's original index, got index=%d ok=%v", index, ok)
	}
	mdl.Type("zzz") // matches nothing
	if _, ok := mdl.SelectedIndex(); ok {
		t.Fatal("SelectedIndex must report ok=false when nothing matches")
	}
}

// An empty picker is safe: no matches, no selection, cursor pinned at 0.
func TestEmptyPicker(t *testing.T) {
	mdl := New(nil)
	if len(mdl.Visible()) != 0 {
		t.Fatalf("no apps -> nothing visible, got %d", len(mdl.Visible()))
	}
	mdl.MoveCursor(1)
	if mdl.Cursor() != 0 {
		t.Fatalf("cursor stays 0 with no apps, got %d", mdl.Cursor())
	}
	if _, ok := mdl.Selected(); ok {
		t.Fatal("no apps -> Selected ok=false")
	}
}

// MoveCursor clamps within the NARROWED list, not just the full one.
func TestMoveCursor_ClampsWithinNarrowedList(t *testing.T) {
	mdl := New(sampleApps())
	mdl.Type("t") // alacritty and syncthing contain a 't'; firefox does not
	visible := len(mdl.Visible())
	if visible == 0 || visible == len(sampleApps()) {
		t.Fatalf("precondition: 't' should narrow but not empty, got %d", visible)
	}
	mdl.MoveCursor(1000)
	if mdl.Cursor() != visible-1 {
		t.Fatalf("cursor should clamp to the last narrowed row %d, got %d", visible-1, mdl.Cursor())
	}
}

// Selected tracks a non-zero cursor to the right app.
func TestSelected_TracksNonZeroCursor(t *testing.T) {
	mdl := New(sampleApps()) // caller order: alacritty, firefox, syncthing
	mdl.MoveCursor(2)
	app, ok := mdl.Selected()
	if !ok || app.Name != "syncthing" {
		t.Fatalf("cursor 2 should select syncthing, got %q ok=%v", app.Name, ok)
	}
}

func names(apps []App) []string {
	out := make([]string, len(apps))
	for index, app := range apps {
		out[index] = app.Name
	}
	return out
}

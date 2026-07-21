// Package picker is zlg's toolkit-neutral view-model: the state of the app picker (the
// filtered list, the query, the cursor, the running set) and the transitions a UI drives
// (type, backspace, clear, move, select). It holds no gio types and does no I/O, so the
// launch/filter logic is unit-testable without a display - the gio layer (internal/ui) is
// a thin renderer over this, exactly as zlt's Bubbletea model is over the same primitives.
//
// The behaviour mirrors zlt so the two launchers feel identical: fuzzy filter as you type
// (best match first), the cursor clamps to the visible range and resets to the top on a
// new query, and a running indicator comes from a name set the caller supplies.
package picker

import (
	"github.com/crispuscrew/zinc/launcher/common/match"
)

// App is one launchable entry as the picker shows it.
type App struct {
	Name        string
	Description string
	Running     bool
}

// Model is the picker state. Construct it with New; the UI reads Query / Visible / Cursor
// to render and calls the transition methods on input.
type Model struct {
	apps     []App          // every defined app, in the caller's order (alphabetical)
	names    []string       // apps' names, parallel to apps, for the matcher
	filtered []match.Ranked // the current query's matches, best-first (Index into apps)
	query    string         // the filter text
	cursor   int            // selected row, an index into filtered
}

// New builds a picker over apps (which the caller sorts). It filters once for the empty
// query, so Visible is the full list until the user types.
func New(apps []App) *Model {
	names := make([]string, len(apps))
	for index, app := range apps {
		names[index] = app.Name
	}
	mdl := &Model{apps: apps, names: names}
	mdl.refilter()
	return mdl
}

// Query is the current filter text.
func (mdl *Model) Query() string { return mdl.query }

// Type appends text (one or more runes) to the query and refilters.
func (mdl *Model) Type(text string) {
	if text == "" {
		return
	}
	mdl.query += text
	mdl.refilter()
}

// Backspace removes the last rune of the query and refilters. It is a no-op on an empty
// query.
func (mdl *Model) Backspace() {
	if mdl.query == "" {
		return
	}
	runes := []rune(mdl.query)
	mdl.query = string(runes[:len(runes)-1])
	mdl.refilter()
}

// ClearQuery empties the query and refilters (the whole list returns).
func (mdl *Model) ClearQuery() {
	mdl.query = ""
	mdl.refilter()
}

// MoveCursor shifts the selection by delta, clamped to the filtered range (and pinned to 0
// when nothing matches).
func (mdl *Model) MoveCursor(delta int) {
	if len(mdl.filtered) == 0 {
		mdl.cursor = 0
		return
	}
	mdl.cursor += delta
	if mdl.cursor < 0 {
		mdl.cursor = 0
	}
	if mdl.cursor > len(mdl.filtered)-1 {
		mdl.cursor = len(mdl.filtered) - 1
	}
}

// Cursor is the selected row, an index into Visible.
func (mdl *Model) Cursor() int { return mdl.cursor }

// Visible is the filtered apps in ranked (best-first) order - the rows the UI draws.
func (mdl *Model) Visible() []App {
	visible := make([]App, len(mdl.filtered))
	for pos, ranked := range mdl.filtered {
		visible[pos] = mdl.apps[ranked.Index]
	}
	return visible
}

// Selected returns the highlighted app, or ok=false when nothing matches (so the UI knows
// there is nothing to launch).
func (mdl *Model) Selected() (App, bool) {
	if len(mdl.filtered) == 0 {
		return App{}, false
	}
	return mdl.apps[mdl.filtered[mdl.cursor].Index], true
}

// SetRunning marks each named app running (and every other app not running), from the set
// zcr reports. Names not present are left un-running.
func (mdl *Model) SetRunning(running map[string]bool) {
	for index := range mdl.apps {
		mdl.apps[index].Running = running[mdl.apps[index].Name]
	}
}

// refilter recomputes the visible list for the current query and resets the selection to
// the top (the best match).
func (mdl *Model) refilter() {
	mdl.filtered = match.Filter(mdl.query, mdl.names)
	mdl.cursor = 0
}

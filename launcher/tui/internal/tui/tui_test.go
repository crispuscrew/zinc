package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeRunner records launches and serves a canned running set, so Update can be driven
// without a real zcr.
type fakeRunner struct {
	running   map[string]bool
	launched  []string
	launchErr error
}

func (fake *fakeRunner) Launch(name string) error {
	fake.launched = append(fake.launched, name)
	return fake.launchErr
}
func (fake *fakeRunner) Running() (map[string]bool, error) { return fake.running, nil }

func sampleApps() []App {
	return []App{
		{Name: "alacritty", Description: "terminal"},
		{Name: "firefox", Description: "browser"},
		{Name: "syncthing", Description: "sync"},
	}
}

func typeStr(mdl Model, text string) Model {
	for _, chr := range text {
		next, _ := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{chr}})
		mdl = next.(Model)
	}
	return mdl
}

func pressKey(mdl Model, keyType tea.KeyType) Model {
	next, _ := mdl.Update(tea.KeyMsg{Type: keyType})
	return next.(Model)
}

// Typing narrows the list to matching apps.
func TestFilter_NarrowsList(t *testing.T) {
	mdl := typeStr(New(sampleApps(), &fakeRunner{}), "fire")
	if len(mdl.filtered) != 1 {
		t.Fatalf("query 'fire' should match one app, got %d", len(mdl.filtered))
	}
	if got := mdl.apps[mdl.filtered[0].Index].Name; got != "firefox" {
		t.Fatalf("query 'fire' matched %q, want firefox", got)
	}
	// backspacing back to empty restores the full list
	for range "fire" {
		mdl = pressKey(mdl, tea.KeyBackspace)
	}
	if len(mdl.filtered) != 3 {
		t.Fatalf("clearing the query should restore all 3, got %d", len(mdl.filtered))
	}
}

// The cursor moves within the filtered range and clamps at the ends.
func TestCursor_MovesAndClamps(t *testing.T) {
	mdl := New(sampleApps(), &fakeRunner{}) // 3 apps, cursor 0
	mdl = pressKey(mdl, tea.KeyDown)
	if mdl.cursor != 1 {
		t.Fatalf("down -> cursor 1, got %d", mdl.cursor)
	}
	mdl = pressKey(mdl, tea.KeyDown)
	mdl = pressKey(mdl, tea.KeyDown) // clamp at last (index 2)
	if mdl.cursor != 2 {
		t.Fatalf("down past end -> cursor clamps at 2, got %d", mdl.cursor)
	}
	mdl = pressKey(mdl, tea.KeyUp)
	if mdl.cursor != 1 {
		t.Fatalf("up -> cursor 1, got %d", mdl.cursor)
	}
}

// Enter launches the selected app through the runner and quits.
func TestEnter_LaunchesSelectedAndQuits(t *testing.T) {
	fake := &fakeRunner{}
	mdl := typeStr(New(sampleApps(), fake), "fire") // selects firefox

	updated, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mdl = updated.(Model)
	if cmd == nil {
		t.Fatal("enter should return a launch command")
	}
	msg := cmd() // runs runner.Launch and yields a launchedMsg
	updated, quit := mdl.Update(msg)
	mdl = updated.(Model)

	if len(fake.launched) != 1 || fake.launched[0] != "firefox" {
		t.Fatalf("expected firefox launched once, got %v", fake.launched)
	}
	if mdl.Launched() != "firefox" {
		t.Fatalf("Launched() = %q, want firefox", mdl.Launched())
	}
	if quit == nil {
		t.Fatal("a successful launch should quit the picker")
	}
}

// A launch failure is shown and the picker stays open (not launched).
func TestEnter_LaunchErrorStaysOpen(t *testing.T) {
	fake := &fakeRunner{launchErr: errors.New("boom")}
	mdl := typeStr(New(sampleApps(), fake), "fire")

	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated, _ := mdl.Update(cmd())
	mdl = updated.(Model)

	if mdl.Launched() != "" {
		t.Fatal("a failed launch must not set Launched()")
	}
	if !strings.Contains(mdl.status, "boom") {
		t.Fatalf("the error should show in the status, got %q", mdl.status)
	}
}

// The running-state message marks the matching app.
func TestRunningMsg_MarksApp(t *testing.T) {
	updated, _ := New(sampleApps(), &fakeRunner{}).Update(runningMsg{running: map[string]bool{"firefox": true}})
	mdl := updated.(Model)
	for _, app := range mdl.apps {
		if app.Name == "firefox" && !app.Running {
			t.Fatal("firefox should be marked running")
		}
		if app.Name == "alacritty" && app.Running {
			t.Fatal("alacritty should not be marked running")
		}
	}
}

// Enter with no matches is a safe no-op: nothing launches, no out-of-range access.
func TestEnter_NoMatchesIsNoOp(t *testing.T) {
	fake := &fakeRunner{}
	mdl := typeStr(New(sampleApps(), fake), "zzz")
	if len(mdl.filtered) != 0 {
		t.Fatalf("precondition: 'zzz' should match nothing, got %d", len(mdl.filtered))
	}
	updated, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mdl = updated.(Model)
	if cmd != nil {
		t.Fatal("enter with no matches should be a no-op (nil command)")
	}
	if len(fake.launched) != 0 || mdl.Launched() != "" {
		t.Fatal("enter with no matches must not launch anything")
	}
}

// ctrl+u clears the filter; ctrl+n/ctrl+p navigate like the arrows.
func TestCtrlKeys(t *testing.T) {
	mdl := typeStr(New(sampleApps(), &fakeRunner{}), "fire")
	mdl = pressKey(mdl, tea.KeyCtrlU)
	if mdl.query != "" || len(mdl.filtered) != 3 {
		t.Fatalf("ctrl+u should clear the filter, got query=%q filtered=%d", mdl.query, len(mdl.filtered))
	}
	mdl = pressKey(mdl, tea.KeyCtrlN)
	if mdl.cursor != 1 {
		t.Fatalf("ctrl+n -> cursor 1, got %d", mdl.cursor)
	}
	mdl = pressKey(mdl, tea.KeyCtrlP)
	if mdl.cursor != 0 {
		t.Fatalf("ctrl+p -> cursor 0, got %d", mdl.cursor)
	}
}

// The scroll window keeps the cursor visible and always returns in-bounds indices.
func TestWindow_KeepsCursorVisibleInBounds(t *testing.T) {
	apps := make([]App, 30)
	for i := range apps {
		apps[i] = App{Name: fmt.Sprintf("app%02d", i)}
	}
	mdl := New(apps, &fakeRunner{})
	mdl.height = 10 // rows = height - 5 = 5
	mdl.cursor = 25

	start, end := mdl.window()
	if !(start <= mdl.cursor && mdl.cursor < end) {
		t.Fatalf("window [%d,%d) should contain cursor %d", start, end, mdl.cursor)
	}
	if start < 0 || end > len(mdl.filtered) {
		t.Fatalf("window [%d,%d) out of bounds (len %d)", start, end, len(mdl.filtered))
	}
	if end-start > 5 {
		t.Fatalf("window shows %d rows, want <= 5", end-start)
	}
}

// The empty-store view guides the user to create an app; a populated view lists names.
func TestView(t *testing.T) {
	if out := New(nil, &fakeRunner{}).View(); !strings.Contains(out, "no apps defined") {
		t.Fatalf("empty view should guide the user, got:\n%s", out)
	}
	if out := New(sampleApps(), &fakeRunner{}).View(); !strings.Contains(out, "firefox") {
		t.Fatalf("populated view should list apps, got:\n%s", out)
	}
}

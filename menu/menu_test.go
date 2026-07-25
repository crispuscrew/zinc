package menu

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/menu/internal/picker"
	"github.com/rajveermalviya/go-wayland/wayland/client"
)

// newTestApp builds an app with no Wayland connection. Every method exercised below tolerates
// that: redraw returns immediately while there is no buffer, so the activation state machine
// can be driven without a compositor.
func newTestApp(activate ActivateFunc) *app {
	items := []Item{{Label: "alpha"}, {Label: "beta"}}
	return &app{
		model:    picker.New(toApps(items)),
		items:    items,
		activate: activate,
		busyVerb: "launching",
		selected: -1,
	}
}

// Enter must not run the activation on the event loop. Before the fix the callback ran inline,
// so the overlay stopped redrawing and held its exclusive keyboard grab for the whole launch -
// seconds for a container, minutes when a derived image had to be built.
func TestActivateSelected_DoesNotBlockTheEventLoop(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	application := newTestApp(func(Item) error {
		close(started)
		<-release
		return nil
	})

	done := make(chan struct{})
	go func() { application.activateSelected(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("activateSelected blocked on the activation instead of returning to the event loop")
	}
	<-started
	if !application.activating {
		t.Error("activating should be set while the callback runs")
	}
	if application.closed {
		t.Error("the menu must stay open while the activation runs")
	}
	close(release)
}

// A second Enter while one activation is in flight must be ignored. Two concurrent launches of
// one app race on the same pod, and the loser's fail-closed teardown removes what the winner
// just created - the same double-launch bug already fixed in zlt.
func TestActivateSelected_IgnoresASecondEnter(t *testing.T) {
	release := make(chan struct{})
	calls := make(chan Item, 4)
	application := newTestApp(func(item Item) error {
		calls <- item
		<-release
		return nil
	})

	application.activateSelected()
	application.activateSelected()
	application.activateSelected()
	close(release)

	if err := application.awaitActivate(); err != nil {
		t.Fatalf("awaitActivate: %v", err)
	}
	close(calls)
	if count := len(calls); count != 1 {
		t.Fatalf("activate was called %d times, want exactly 1", count)
	}
}

// A finished activation is collected on the frame callback: success closes the menu with that
// item selected, failure keeps it open with the error in the banner.
func TestOnFrame_CollectsTheActivationResult(t *testing.T) {
	for _, tc := range []struct {
		name        string
		activateErr error
		wantClosed  bool
		wantSel     int
		wantErrText string
	}{
		{name: "success", wantClosed: true, wantSel: 0},
		{name: "failure", activateErr: errors.New("no such app"), wantClosed: false, wantSel: -1, wantErrText: "no such app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			application := newTestApp(func(Item) error { return tc.activateErr })
			application.activateSelected()

			// Poll frames the way the compositor would until the result lands.
			deadline := time.Now().Add(2 * time.Second)
			for application.activating && time.Now().Before(deadline) {
				application.onFrame(client.CallbackDoneEvent{CallbackData: 1})
			}
			if application.activating {
				t.Fatal("the activation result was never collected")
			}
			if application.closed != tc.wantClosed {
				t.Errorf("closed = %v, want %v", application.closed, tc.wantClosed)
			}
			if application.selected != tc.wantSel {
				t.Errorf("selected = %d, want %d", application.selected, tc.wantSel)
			}
			if application.launchErr != tc.wantErrText {
				t.Errorf("launchErr = %q, want %q", application.launchErr, tc.wantErrText)
			}
		})
	}
}

// Dismissing the overlay mid-activation must not strand the callback: Run tears the window down
// and then waits, so nothing is still running behind a caller that has already got Run's return.
// A launch that succeeded still counts as activated - the work happened, the user only stopped
// watching it.
func TestAwaitActivate_WaitsForADismissedActivation(t *testing.T) {
	release := make(chan struct{})
	finished := false
	application := newTestApp(func(Item) error {
		<-release
		finished = true
		return nil
	})
	application.activateSelected()

	// Esc, i.e. what handleKey does, then Run's post-loop wait.
	application.closed = true
	go func() { time.Sleep(20 * time.Millisecond); close(release) }()
	if err := application.awaitActivate(); err != nil {
		t.Fatalf("awaitActivate: %v", err)
	}
	if !finished {
		t.Error("awaitActivate returned before the activation finished")
	}
	if application.selected != 0 {
		t.Errorf("selected = %d, want 0: the activation did succeed", application.selected)
	}
}

// With nothing in flight the wait is a no-op, so a plain Esc returns immediately.
func TestAwaitActivate_NoOpWhenIdle(t *testing.T) {
	application := newTestApp(nil)
	if err := application.awaitActivate(); err != nil {
		t.Fatalf("awaitActivate on an idle menu: %v", err)
	}
}

// A nil ActivateFunc keeps the old behaviour: Enter just closes the menu with the selection.
func TestActivateSelected_NilActivateClosesImmediately(t *testing.T) {
	application := newTestApp(nil)
	application.activateSelected()
	if !application.closed || application.selected != 0 {
		t.Fatalf("closed=%v selected=%d, want true/0", application.closed, application.selected)
	}
	if application.activating {
		t.Error("a nil activate should not put the menu in the activating state")
	}
}

// The busy banner names the item and cycles its ellipsis with the compositor's frame clock, so
// a long activation reads as working rather than as a frozen window.
func TestBusyBanner(t *testing.T) {
	application := newTestApp(nil)
	if got := application.busyBanner(); got != "" {
		t.Errorf("idle banner = %q, want empty", got)
	}
	application.activating = true
	application.activateItem = "nvim"

	seen := map[string]bool{}
	for step := 0; step < busyDotMax; step++ {
		application.frameMs = uint32(step * busyDotMs)
		banner := application.busyBanner()
		if !strings.HasPrefix(banner, "launching nvim") {
			t.Fatalf("banner = %q, want it to name the verb and the item", banner)
		}
		seen[banner] = true
	}
	if len(seen) != busyDotMax {
		t.Errorf("the ellipsis produced %d distinct frames, want %d", len(seen), busyDotMax)
	}
}

// toApps maps public items to internal picker rows field-for-field, in order, and resolves an
// empty Icon spec to no icon.
func TestToApps_MapsFieldsAndOrder(t *testing.T) {
	items := []Item{
		{Label: "firefox", Description: "browser", Group: "Web", Preview: "/w/a.jpg", Marked: true},
		{Label: "htop", Description: "monitor", Group: "System"},
	}
	apps := toApps(items)
	if len(apps) != len(items) {
		t.Fatalf("toApps returned %d rows, want %d", len(apps), len(items))
	}
	for index, app := range apps {
		item := items[index]
		if app.Name != item.Label || app.Description != item.Description || app.Group != item.Group {
			t.Errorf("row %d = {%q,%q,%q}, want {%q,%q,%q}", index, app.Name, app.Description, app.Group, item.Label, item.Description, item.Group)
		}
		if app.Preview != item.Preview {
			t.Errorf("row %d Preview = %q, want %q", index, app.Preview, item.Preview)
		}
		if app.Running != item.Marked {
			t.Errorf("row %d Running = %v, want Marked %v", index, app.Running, item.Marked)
		}
		if app.Icon != nil {
			t.Errorf("row %d Icon should be nil for an empty Icon spec", index)
		}
	}
}

// Up/Down step one row: a single item in the list, a whole column-count in the grid, so the
// vertical arrows move between rows rather than to the immediate neighbour.
func TestRowStep_ListVsGrid(t *testing.T) {
	list := &app{}
	if got := list.rowStep(); got != 1 {
		t.Fatalf("list rowStep = %d, want 1", got)
	}
	grid := &app{grid: true, width: 920, cellW: 180}
	if got := grid.rowStep(); got < 2 {
		t.Fatalf("grid rowStep = %d, want the column count (>1 for a wide overlay)", got)
	}
}

func TestOptionDefaults(t *testing.T) {
	if orString("", "fallback") != "fallback" || orString("set", "fallback") != "set" {
		t.Error("orString did not apply the fallback correctly")
	}
	if orInt(0, 720) != 720 || orInt(-5, 720) != 720 || orInt(500, 720) != 500 {
		t.Error("orInt did not apply the fallback correctly")
	}
}

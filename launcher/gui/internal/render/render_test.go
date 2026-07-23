package render

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/crispuscrew/zinc/launcher/gui/internal/picker"
	"github.com/crispuscrew/zinc/launcher/gui/internal/theme"
)

var pal = theme.Dark()

func sample() *picker.Model {
	return picker.New([]picker.App{
		{Name: "alacritty", Description: "terminal"},
		{Name: "firefox", Description: "browser", Running: true},
		{Name: "syncthing", Description: "sync"},
	})
}

func TestFrame_Size(t *testing.T) {
	img := Frame(sample(), pal, 400, 300)
	if got := img.Bounds(); got.Dx() != 400 || got.Dy() != 300 {
		t.Fatalf("frame is %dx%d, want 400x300", got.Dx(), got.Dy())
	}
}

// Something is drawn: at least one pixel differs from the background fill.
func TestFrame_DrawsContent(t *testing.T) {
	img := Frame(sample(), pal, 400, 300)
	if !hasColor(img, pal.SelFG) {
		t.Fatal("the selected row's foreground text should appear")
	}
	if !hasColor(img, pal.Running) {
		t.Fatal("firefox is running, so a running dot should be drawn")
	}
}

// The selected row (cursor 0 by default) sits on the highlight band.
func TestFrame_SelectionBand(t *testing.T) {
	img := Frame(sample(), pal, 400, 300)
	if !hasColor(img, pal.SelBG) {
		t.Fatal("the selected row should have the highlight background")
	}
}

func TestFrame_NoMatchesDoesNotPanic(t *testing.T) {
	mdl := sample()
	mdl.Type("zzzzz") // matches nothing
	img := Frame(mdl, pal, 400, 300)
	if img.Bounds().Dx() != 400 {
		t.Fatal("frame should still render at full size with no matches")
	}
}

func TestFrame_EmptyAndTinyAreSafe(t *testing.T) {
	// no apps
	_ = Frame(picker.New(nil), pal, 200, 120)
	// a size too small for even one row must clamp, not divide-by-zero or panic
	_ = Frame(sample(), pal, 50, 10)
}

// scrollStart keeps the cursor visible and the window in-bounds for every cursor position
// when the list is longer than the window - the scroll-active path the size tests skip.
func TestScrollStart_KeepsCursorVisibleInBounds(t *testing.T) {
	const total, rows = 30, 5
	for cursor := 0; cursor < total; cursor++ {
		start := scrollStart(cursor, total, rows)
		if start < 0 || start+rows > total {
			t.Fatalf("cursor %d: window [%d,%d) out of [0,%d]", cursor, start, start+rows, total)
		}
		if cursor < start || cursor >= start+rows {
			t.Fatalf("cursor %d not visible in window [%d,%d)", cursor, start, start+rows)
		}
	}
}

// A frame whose cursor scrolled past the first page still draws the selected row's
// highlight band, so the selection stays on screen rather than scrolling off.
func TestFrame_ScrolledCursorStaysHighlighted(t *testing.T) {
	apps := make([]picker.App, 30)
	for index := range apps {
		apps[index] = picker.App{Name: fmt.Sprintf("app%02d", index)}
	}
	mdl := picker.New(apps)
	for count := 0; count < 29; count++ {
		mdl.MoveCursor(1) // drive the cursor to the last row
	}
	img := Frame(mdl, pal, 400, 200) // a short window so the 30 rows must scroll
	if !hasColor(img, pal.SelBG) {
		t.Fatal("the scrolled-to selected row should still show the highlight band")
	}
}

func hasColor(img *image.RGBA, want color.Color) bool {
	// compare RGBA tuples pixel by pixel against a palette colour.
	wr, wg, wb, wa := want.RGBA()
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r == wr && g == wg && b == wb && a == wa {
				return true
			}
		}
	}
	return false
}

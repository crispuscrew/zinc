package render

import (
	"fmt"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/crispuscrew/zinc/menu/internal/picker"
	"github.com/crispuscrew/zinc/menu/internal/theme"
)

var pal = theme.Dark()

// fullView is a fully-shown, opaque frame (no fade, no launch error), the common case.
var fullView = View{Fade: 1, Opacity: 1}

func sample() *picker.Model {
	return picker.New([]picker.App{
		{Name: "alacritty", Description: "terminal"},
		{Name: "firefox", Description: "browser", Running: true},
		{Name: "syncthing", Description: "sync"},
	})
}

func TestFrame_Size(t *testing.T) {
	img := Frame(sample(), pal, fullView, 400, 300)
	if got := img.Bounds(); got.Dx() != 400 || got.Dy() != 300 {
		t.Fatalf("frame is %dx%d, want 400x300", got.Dx(), got.Dy())
	}
}

// Something is drawn: at least one pixel differs from the background fill.
func TestFrame_DrawsContent(t *testing.T) {
	img := Frame(sample(), pal, fullView, 400, 300)
	// The running dot is a solid fill, so it lands on an exact palette color.
	if !hasColor(img, pal.Running) {
		t.Fatal("firefox is running, so a running dot should be drawn")
	}
	// Antialiased text does not sit on exact palette colors, so assert glyphs were drawn by
	// finding pixels in the first row that are none of the background or its solid fills.
	if !hasGlyphPixels(img, headerH+4, headerH+4+rowH) {
		t.Fatal("the first row's name text should draw glyph pixels")
	}
}

// hasGlyphPixels reports whether the horizontal band [y0,y1) holds any opaque pixel that is
// not the background, the selection band, or the accent bar - i.e. antialiased text.
func hasGlyphPixels(img *image.RGBA, y0, y1 int) bool {
	solids := []color.RGBA{pal.BG, pal.SelBG, pal.Accent}
	for y := y0; y < y1; y++ {
		for x := nameX; x < img.Bounds().Dx()-marginX; x++ {
			got := img.RGBAAt(x, y)
			if got.A == 0 {
				continue
			}
			isSolid := false
			for _, solid := range solids {
				if got == solid {
					isSolid = true
					break
				}
			}
			if !isSolid {
				return true
			}
		}
	}
	return false
}

// The selected row (cursor 0 by default) sits on the highlight band.
func TestFrame_SelectionBand(t *testing.T) {
	img := Frame(sample(), pal, fullView, 400, 300)
	if !hasColor(img, pal.SelBG) {
		t.Fatal("the selected row should have the highlight background")
	}
}

// A launch error is shown in the window (in the error color), not swallowed.
func TestFrame_ErrorBanner(t *testing.T) {
	view := View{Fade: 1, Opacity: 1, Error: "launch neovim: zcr not found"}
	img := Frame(sample(), pal, view, 400, 300)
	if !hasColor(img, pal.Error) {
		t.Fatal("a launch error should draw the error banner in the error color")
	}
}

// Mid-fade (fade 0) the whole surface is transparent, so nothing flashes opaque.
func TestFrame_FadeZeroIsFullyTransparent(t *testing.T) {
	img := Frame(sample(), pal, View{Fade: 0, Opacity: 1}, 400, 300)
	if got := img.RGBAAt(200, 150).A; got != 0 {
		t.Fatalf("center pixel alpha = %d at fade 0, want 0 (fully transparent)", got)
	}
}

// The corners are rounded: the very corner pixel is transparent even when fully shown.
func TestFrame_CornersAreRounded(t *testing.T) {
	img := Frame(sample(), pal, fullView, 400, 300)
	if got := img.RGBAAt(0, 0).A; got != 0 {
		t.Fatalf("top-left corner alpha = %d, want 0 (rounded, transparent)", got)
	}
	if got := img.RGBAAt(200, 150).A; got != 0xff {
		t.Fatalf("center alpha = %d, want 255 (opaque body)", got)
	}
}

func TestFrame_NoMatchesDoesNotPanic(t *testing.T) {
	mdl := sample()
	mdl.Type("zzzzz") // matches nothing
	img := Frame(mdl, pal, fullView, 400, 300)
	if img.Bounds().Dx() != 400 {
		t.Fatal("frame should still render at full size with no matches")
	}
}

func TestFrame_EmptyAndTinyAreSafe(t *testing.T) {
	// no apps
	_ = Frame(picker.New(nil), pal, fullView, 200, 120)
	// a size too small for even one row must clamp, not divide-by-zero or panic
	_ = Frame(sample(), pal, fullView, 50, 10)
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
	img := Frame(mdl, pal, fullView, 400, 200) // a short window so the 30 rows must scroll
	if !hasColor(img, pal.SelBG) {
		t.Fatal("the scrolled-to selected row should still show the highlight band")
	}
}

// buildRows inserts a header before each new group (empty group becomes "Other"), keeps every
// item, and rowForItem maps each item to its own non-header row.
func TestBuildRows_GroupsInsertHeaders(t *testing.T) {
	visible := []picker.App{
		{Name: "firefox", Group: "Web"},
		{Name: "chromium", Group: "Web"},
		{Name: "htop", Group: "Dev"},
		{Name: "misc", Group: ""},
	}
	rows := buildRows(visible, true)
	var headers []string
	items := 0
	for _, row := range rows {
		if row.isHeader {
			headers = append(headers, row.header)
		} else {
			items++
		}
	}
	if items != len(visible) {
		t.Fatalf("item rows = %d, want %d", items, len(visible))
	}
	if want := []string{"Web", "Dev", "Other"}; !reflect.DeepEqual(headers, want) {
		t.Fatalf("headers = %v, want %v", headers, want)
	}
	for index := range visible {
		at := rowForItem(rows, index)
		if rows[at].isHeader || rows[at].item != index {
			t.Fatalf("rowForItem(%d) landed on header=%v item=%d", index, rows[at].isHeader, rows[at].item)
		}
	}
}

// Without grouping the rows are the plain items, one per index, no headers.
func TestBuildRows_FlatWhenNotGrouping(t *testing.T) {
	visible := []picker.App{{Name: "a", Group: "X"}, {Name: "b", Group: "Y"}}
	rows := buildRows(visible, false)
	if len(rows) != len(visible) {
		t.Fatalf("flat rows = %d, want %d", len(rows), len(visible))
	}
	for index, row := range rows {
		if row.isHeader || row.item != index {
			t.Fatalf("row %d should be a plain item", index)
		}
	}
}

// An idle model with groups renders the header path without panicking.
func TestFrame_GroupedHeadersDoNotPanic(t *testing.T) {
	mdl := picker.New([]picker.App{
		{Name: "firefox", Group: "Web"},
		{Name: "htop", Group: "Dev"},
	})
	_ = Frame(mdl, pal, fullView, 400, 300)
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

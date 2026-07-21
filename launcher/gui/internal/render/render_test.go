package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/crispuscrew/zinc/launcher/gui/internal/picker"
)

func sample() *picker.Model {
	return picker.New([]picker.App{
		{Name: "alacritty", Description: "terminal"},
		{Name: "firefox", Description: "browser", Running: true},
		{Name: "syncthing", Description: "sync"},
	})
}

func TestFrame_Size(t *testing.T) {
	img := Frame(sample(), 400, 300)
	if got := img.Bounds(); got.Dx() != 400 || got.Dy() != 300 {
		t.Fatalf("frame is %dx%d, want 400x300", got.Dx(), got.Dy())
	}
}

// Something is drawn: at least one pixel differs from the background fill.
func TestFrame_DrawsContent(t *testing.T) {
	img := Frame(sample(), 400, 300)
	if !hasColor(img, colorSelFG) {
		t.Fatal("the selected row's foreground text should appear")
	}
	if !hasColor(img, colorRunning) {
		t.Fatal("firefox is running, so a running dot should be drawn")
	}
}

// The selected row (cursor 0 by default) sits on the highlight band.
func TestFrame_SelectionBand(t *testing.T) {
	img := Frame(sample(), 400, 300)
	if !hasColor(img, colorSelBG) {
		t.Fatal("the selected row should have the highlight background")
	}
}

func TestFrame_NoMatchesDoesNotPanic(t *testing.T) {
	mdl := sample()
	mdl.Type("zzzzz") // matches nothing
	img := Frame(mdl, 400, 300)
	if img.Bounds().Dx() != 400 {
		t.Fatal("frame should still render at full size with no matches")
	}
}

func TestFrame_EmptyAndTinyAreSafe(t *testing.T) {
	// no apps
	_ = Frame(picker.New(nil), 200, 120)
	// a size too small for even one row must clamp, not divide-by-zero or panic
	_ = Frame(sample(), 50, 10)
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

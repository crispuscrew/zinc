package icons

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_AbsolutePathDecodesAndScales(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "icon.png")
	source := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			source.Set(x, y, color.RGBA{0x10, 0x80, 0xf0, 0xff})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, source); err != nil {
		t.Fatal(err)
	}
	file.Close()

	got := Resolve(path, 16)
	if got == nil {
		t.Fatal("resolving an existing PNG path returned nil")
	}
	if bounds := got.Bounds(); bounds.Dx() != 16 || bounds.Dy() != 16 {
		t.Fatalf("scaled icon is %dx%d, want 16x16", bounds.Dx(), bounds.Dy())
	}
}

// A name with nothing on disk, an empty spec, and a missing path all resolve to nil. The
// search roots are pointed at empty temp dirs so the result does not depend on the host's
// installed icon themes.
func TestResolve_MissingAndEmptyReturnNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	if got := Resolve("definitely-not-a-real-icon-xyz", 16); got != nil {
		t.Fatal("an unknown icon name should resolve to nil")
	}
	if got := Resolve("", 16); got != nil {
		t.Fatal("an empty spec should resolve to nil")
	}
	if got := Resolve(filepath.Join(t.TempDir(), "nope.png"), 16); got != nil {
		t.Fatal("a missing absolute path should resolve to nil")
	}
}

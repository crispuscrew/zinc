package imgutil

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// writePNG encodes a w x h solid image to a temp file and returns its path.
func writePNG(t *testing.T, dir string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{0x20, 0x60, 0xa0, 0xff})
		}
	}
	path := filepath.Join(dir, "img.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDecode_ValidFile(t *testing.T) {
	path := writePNG(t, t.TempDir(), 40, 30)
	got := Decode(path, 8<<20, 4<<20)
	if got == nil {
		t.Fatal("decoding a valid PNG returned nil")
	}
	if b := got.Bounds(); b.Dx() != 40 || b.Dy() != 30 {
		t.Fatalf("decoded %dx%d, want 40x30", b.Dx(), b.Dy())
	}
}

// A directory, a missing path, garbage bytes, and a byte cap smaller than the file all decode
// to nil rather than panicking or blocking - the path is partly-untrusted.
func TestDecode_RejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	if got := Decode(dir, 8<<20, 4<<20); got != nil {
		t.Error("a directory should decode to nil")
	}
	if got := Decode(filepath.Join(dir, "missing.png"), 8<<20, 4<<20); got != nil {
		t.Error("a missing path should decode to nil")
	}
	garbage := filepath.Join(dir, "x.png")
	if err := os.WriteFile(garbage, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Decode(garbage, 8<<20, 4<<20); got != nil {
		t.Error("garbage bytes should decode to nil")
	}
}

// A pixel budget below the image's declared size rejects it before allocating the decoder's
// buffers (the OOM guard), so an oversized image decodes to nil.
func TestDecode_PixelCapRejectsOversize(t *testing.T) {
	path := writePNG(t, t.TempDir(), 100, 100) // 10_000 px
	if got := Decode(path, 8<<20, 100); got != nil {
		t.Fatal("an image past the pixel cap should decode to nil")
	}
}

func TestSquare(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	got := Square(src, 16)
	if got == nil || got.Bounds().Dx() != 16 || got.Bounds().Dy() != 16 {
		t.Fatalf("Square = %v, want 16x16", got.Bounds())
	}
	if Square(nil, 16) != nil {
		t.Error("Square(nil) should be nil")
	}
	if Square(src, 0) != nil {
		t.Error("Square(_, 0) should be nil")
	}
}

// Fit scales to touch one box edge and preserves aspect ratio (never stretches or crops).
func TestFit_PreservesAspectWithinBox(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 200)) // 2:1
	got := Fit(src, 100, 100)
	if got == nil {
		t.Fatal("Fit returned nil for a valid source")
	}
	// Width-bound: 400x200 into 100x100 -> 100x50 (aspect kept, fits inside the box).
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 50 {
		t.Fatalf("Fit = %dx%d, want 100x50", got.Bounds().Dx(), got.Bounds().Dy())
	}
}

// Fit never upscales: a source smaller than the box comes back at native size.
func TestFit_DoesNotUpscale(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 20, 10))
	got := Fit(src, 200, 200)
	if got.Bounds().Dx() != 20 || got.Bounds().Dy() != 10 {
		t.Fatalf("Fit upscaled to %dx%d, want native 20x10", got.Bounds().Dx(), got.Bounds().Dy())
	}
}

func TestFit_NilAndDegenerate(t *testing.T) {
	if Fit(nil, 100, 100) != nil {
		t.Error("Fit(nil) should be nil")
	}
	if Fit(image.NewRGBA(image.Rect(0, 0, 10, 10)), 0, 100) != nil {
		t.Error("Fit with a zero box dimension should be nil")
	}
}

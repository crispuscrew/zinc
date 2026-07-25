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

// A symlink to an image decodes: the regular-file guard resolves the link rather than
// describing it. Symlinked icons are common under /usr/share/icons, and symlinked wallpaper
// directories are a standard dotfiles pattern, so rejecting links rendered them all blank.
func TestDecode_FollowsSymlinkToRegularFile(t *testing.T) {
	dir := t.TempDir()
	target := writePNG(t, dir, 40, 30)
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	got := Decode(link, 8<<20, 4<<20)
	if got == nil {
		t.Fatal("a symlink to a valid PNG decoded to nil")
	}
	if b := got.Bounds(); b.Dx() != 40 || b.Dy() != 30 {
		t.Fatalf("decoded %dx%d through the symlink, want 40x30", b.Dx(), b.Dy())
	}
}

// Following the link must not weaken the guard: a symlink whose target is a directory (or any
// other non-regular file) is still rejected.
func TestDecode_RejectsSymlinkToNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	if got := Decode(link, 8<<20, 4<<20); got != nil {
		t.Error("a symlink to a directory should decode to nil")
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

// A byte budget below what the image would decode to rejects it before the decoder allocates
// anything (the OOM guard), so an oversized image decodes to nil.
func TestDecode_ByteCapRejectsOversize(t *testing.T) {
	path := writePNG(t, t.TempDir(), 100, 100) // 10_000 px, 4 bytes each
	if got := Decode(path, 8<<20, 100); got != nil {
		t.Fatal("an image past the decoded-byte cap should decode to nil")
	}
	if got := Decode(path, 8<<20, 10_000*4); got == nil {
		t.Fatal("an image exactly at the budget should still decode")
	}
}

// The budget is charged per decoded byte, not per pixel. A 16-bit-per-channel PNG allocates
// 8 bytes a pixel where an 8-bit one allocates 4, so the same dimensions cost twice as much -
// which is what a pixel cap missed, letting a deep-colour file sail past a limit sized for a
// photograph and allocate several times what the cap implied.
func TestDecode_DeepColourCostsMore(t *testing.T) {
	dir := t.TempDir()
	const width, height = 64, 64
	shallow := writePNG(t, dir, width, height)
	deep := filepath.Join(dir, "deep.png")
	img := image.NewRGBA64(image.Rect(0, 0, width, height))
	file, err := os.Create(deep)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	// A budget that fits the 8-bit image exactly must not fit the 16-bit one of equal size.
	budget := int64(width * height * 4)
	if Decode(shallow, 8<<20, budget) == nil {
		t.Error("the 8-bit image should fit a budget of 4 bytes a pixel")
	}
	if Decode(deep, 8<<20, budget) != nil {
		t.Error("the 16-bit image needs 8 bytes a pixel and must be refused at that budget")
	}
	if Decode(deep, 8<<20, budget*2) == nil {
		t.Error("the 16-bit image should decode once the budget covers 8 bytes a pixel")
	}
}

// Every colour model the registered decoders produce is charged at least what it allocates.
// Under-charging any of them would put a hole in the guard.
func TestBytesPerPixel_NeverUnderCharges(t *testing.T) {
	cases := []struct {
		model color.Model
		want  int64
	}{
		{color.RGBAModel, 4}, {color.NRGBAModel, 4}, {color.CMYKModel, 4}, {color.NYCbCrAModel, 4},
		{color.RGBA64Model, 8}, {color.NRGBA64Model, 8},
		{color.YCbCrModel, 3},
		{color.GrayModel, 1}, {color.Gray16Model, 2},
		{color.AlphaModel, 1}, {color.Alpha16Model, 2},
		{color.Palette{}, 1},
	}
	for _, tc := range cases {
		if got := bytesPerPixel(tc.model); got != tc.want {
			t.Errorf("bytesPerPixel(%T) = %d, want %d", tc.model, got, tc.want)
		}
	}
	// An unrecognised model is charged the widest rate we know of rather than assumed cheap.
	if got := bytesPerPixel(nil); got != 8 {
		t.Errorf("bytesPerPixel(unknown) = %d, want 8", got)
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

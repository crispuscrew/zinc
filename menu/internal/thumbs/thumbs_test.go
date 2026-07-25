package thumbs

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{0x30, 0x70, 0xb0, 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// waitDecoded blocks (up to a generous timeout) until the store has no decode in flight, so a
// background decode has finished. It fails the test rather than hanging forever.
func waitDecoded(t *testing.T, store *Store) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for store.Pending() {
		if time.Now().After(deadline) {
			t.Fatal("thumbnail decode did not finish within the timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A thumbnail that has finished decoding but has not been drawn yet keeps the store Active
// even though nothing is in flight. That is what keeps the caller's frame-callback poll alive
// long enough to blit it: a decode landing after the renderer read the cache but before the
// poll condition was evaluated would otherwise leave nothing armed to draw it, and the tile
// would sit on its placeholder until the next keypress.
func TestActive_StaysSetUntilTheCompletedDecodeIsTaken(t *testing.T) {
	path := writePNG(t, t.TempDir(), "wall.png", 40, 20)
	store := New(100, 100)
	store.Get(path)
	waitDecoded(t, store)

	if store.Pending() {
		t.Fatal("nothing should be in flight once the decode has landed")
	}
	if !store.Active() {
		t.Fatal("a decoded-but-undrawn thumbnail must keep the store Active, or its frame is never drawn")
	}
	if dirty, pending := store.TakeDirty(); !dirty || pending {
		t.Fatalf("TakeDirty = (%v, %v), want (true, false)", dirty, pending)
	}
	if store.Active() {
		t.Fatal("with nothing pending and the dirty flag taken, the store should be idle")
	}
}

// A first Get schedules the decode and returns "not ready"; once the background decode lands,
// the store is dirty and Get returns the box-fitted thumbnail.
func TestGet_DecodesAsynchronously(t *testing.T) {
	path := writePNG(t, t.TempDir(), "wall.png", 400, 200) // 2:1
	store := New(100, 100)

	if thumb, done := store.Get(path); thumb != nil || done {
		t.Fatalf("first Get = (%v, %v), want (nil, false) while scheduling", thumb, done)
	}
	if !store.Pending() {
		t.Fatal("store should report a decode in flight right after the first Get")
	}
	waitDecoded(t, store)
	if dirty, pending := store.TakeDirty(); !dirty || pending {
		t.Fatalf("after the decode landed, TakeDirty = (%v, %v), want (true, false)", dirty, pending)
	}
	if dirty, _ := store.TakeDirty(); dirty {
		t.Fatal("TakeDirty should clear the flag")
	}
	thumb, done := store.Get(path)
	if !done || thumb == nil {
		t.Fatalf("after decoding, Get = (%v, %v), want a thumbnail and done=true", thumb, done)
	}
	// 400x200 fit into a 100x100 box keeps aspect: 100x50.
	if thumb.Bounds().Dx() != 100 || thumb.Bounds().Dy() != 50 {
		t.Fatalf("thumbnail is %dx%d, want 100x50 (aspect-fit)", thumb.Bounds().Dx(), thumb.Bounds().Dy())
	}
}

// A path that cannot decode caches nil ("tried, nothing to draw") and is not rescheduled: Get
// keeps returning (nil, true) and no decode goes back in flight.
func TestGet_FailedDecodeCachesNilOnce(t *testing.T) {
	dir := t.TempDir()
	garbage := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(garbage, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := New(100, 100)
	store.Get(garbage)
	waitDecoded(t, store)

	thumb, done := store.Get(garbage)
	if !done || thumb != nil {
		t.Fatalf("failed decode Get = (%v, %v), want (nil, true)", thumb, done)
	}
	if store.Pending() {
		t.Fatal("a failed path should not be rescheduled")
	}
}

// An empty path is a no-op (done=true, nil) and schedules nothing.
func TestGet_EmptyPath(t *testing.T) {
	store := New(100, 100)
	if thumb, done := store.Get(""); thumb != nil || !done {
		t.Fatalf("Get(\"\") = (%v, %v), want (nil, true)", thumb, done)
	}
	if store.Pending() {
		t.Fatal("an empty path should schedule no decode")
	}
}

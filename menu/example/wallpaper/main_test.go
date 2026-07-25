package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setWallpaper must not wait for the setter to exit. menu calls it from the Wayland event loop
// while the overlay holds an exclusive keyboard grab, so blocking freezes the overlay AND the
// keyboard - and swaybg/hyprpaper are foreground daemons that never exit, which froze it for
// good. It returns once the setter has been given its grace period to fail.
func TestSetWallpaper_DoesNotWaitForALongRunningSetter(t *testing.T) {
	// A script, not "sleep 5": setWallpaper appends the image path to the command, and sleep
	// would reject the extra argument and exit at once - passing the test for the wrong reason.
	// It drops the inherited stdout/stderr first, or the surviving child would hold `go test`'s
	// output pipe open and stall the whole package for the sleep's duration.
	script := filepath.Join(t.TempDir(), "slow-setter")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec >/dev/null 2>&1\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WALLPAPER_CMD", script)

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- setWallpaper("/tmp/x.png") }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a setter that is still running is not a failure, got %v", err)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Fatalf("setWallpaper blocked for %s on a long-running setter", elapsed)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("setWallpaper blocked on a setter that never exits: the overlay would freeze")
	}
}

// A setter that fails immediately is still reported, so a broken recipe reaches the menu's
// error banner instead of looking like success.
func TestSetWallpaper_ReportsAnImmediateFailure(t *testing.T) {
	t.Setenv("WALLPAPER_CMD", "false")
	if err := setWallpaper("/tmp/x.png"); err == nil {
		t.Fatal("a setter exiting non-zero should be reported")
	}

	t.Setenv("WALLPAPER_CMD", "definitely-not-a-real-binary-zzz")
	if err := setWallpaper("/tmp/x.png"); err == nil {
		t.Fatal("a setter that cannot be executed should be reported")
	}
}

// With no setter configured the chooser just prints the path, so it stays compositor-agnostic.
func TestSetWallpaper_UnsetPrintsThePath(t *testing.T) {
	t.Setenv("WALLPAPER_CMD", "")
	if err := setWallpaper("/tmp/x.png"); err != nil {
		t.Fatalf("with no WALLPAPER_CMD, setWallpaper should succeed, got %v", err)
	}
}

// loadWallpapers lists only decodable image files, uses the extension-less name as the label,
// the full path as the thumbnail Preview, and sorts by label.
func TestLoadWallpapers_FiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.png", "a.JPG", "notes.txt", "c.webp"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.png"), 0o755); err != nil {
		t.Fatal(err)
	}

	items, err := loadWallpapers(dir)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
		if item.Preview != filepath.Join(dir, item.Label+filepath.Ext(item.Preview)) {
			t.Errorf("Preview %q should be the full path to the image", item.Preview)
		}
	}
	if len(labels) != 3 || labels[0] != "a" || labels[1] != "b" || labels[2] != "c" {
		t.Fatalf("labels = %v, want [a b c] (txt skipped, directory skipped, sorted)", labels)
	}
}

package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The waiter's actions are injected, so the flock-based "last one out" logic runs
// here with no podman, emulator, or TTY. Each waiter runs in its own goroutine;
// runTerminal blocks on a channel so the test controls close ordering. flock on
// independent open-file-descriptions conflicts even within one process, so the
// in-process markers behave exactly as separate waiter processes would.

func newWaiter(root string, background bool, closeCh <-chan struct{}, stops *int32) *waiter {
	return &waiter{
		runRoot:     root,
		background:  background,
		ensureUp:    func() error { return nil },
		runTerminal: func() error { <-closeCh; return nil },
		stop:        func() error { atomic.AddInt32(stops, 1); return nil },
	}
}

// Two terminals: closing the first must NOT stop the container; closing the last
// must stop it exactly once.
func TestWaiter_LastOneOutStops(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	var stops int32
	first, second := make(chan struct{}), make(chan struct{})
	done := make(chan error, 2)

	go func() { done <- newWaiter(root, false, first, &stops).run("app") }()
	go func() { done <- newWaiter(root, false, second, &stops).run("app") }()
	waitMarkers(t, appDir, 2)

	close(first) // one terminal closes; the other is still open
	if err := <-done; err != nil {
		t.Fatalf("waiter returned error: %v", err)
	}
	if got := atomic.LoadInt32(&stops); got != 0 {
		t.Fatalf("stop must not fire while a terminal is still open, got %d", got)
	}

	close(second) // the last terminal closes
	if err := <-done; err != nil {
		t.Fatalf("waiter returned error: %v", err)
	}
	if got := atomic.LoadInt32(&stops); got != 1 {
		t.Fatalf("last terminal close should stop exactly once, got %d", got)
	}
	if n := countMarkers(appDir); n != 0 {
		t.Fatalf("all markers should be gone after the last close, got %d", n)
	}
}

// A background multiterminal app keeps its holder running after every terminal
// closes - the stop is never called.
func TestWaiter_BackgroundNeverStops(t *testing.T) {
	root := t.TempDir()
	var stops int32
	closeCh := make(chan struct{})
	done := make(chan error, 1)

	go func() { done <- newWaiter(root, true, closeCh, &stops).run("bg") }()
	waitMarkers(t, filepath.Join(root, "bg"), 1)

	close(closeCh)
	if err := <-done; err != nil {
		t.Fatalf("waiter returned error: %v", err)
	}
	if got := atomic.LoadInt32(&stops); got != 0 {
		t.Fatalf("background app must not stop on last close, got %d", got)
	}
}

// A marker left by a crashed terminal (a file nobody flock-holds) must be reaped and
// must not keep the app alive.
func TestWaiter_ReapsStaleMarker(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(appDir, "term.stale")
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	var stops int32
	closeCh := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- newWaiter(root, false, closeCh, &stops).run("app") }()
	waitMarkers(t, appDir, 2) // the stale one + the live waiter's

	close(closeCh)
	if err := <-done; err != nil {
		t.Fatalf("waiter returned error: %v", err)
	}
	if got := atomic.LoadInt32(&stops); got != 1 {
		t.Fatalf("a stale marker must not keep the app alive, want 1 stop, got %d", got)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale marker should have been reaped, stat err: %v", err)
	}
}

func waitMarkers(t *testing.T, dir string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countMarkers(dir) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d markers in %s (have %d)", n, dir, countMarkers(dir))
}

func countMarkers(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "term.") {
			count++
		}
	}
	return count
}

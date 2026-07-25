// Package thumbs is an asynchronous, caching thumbnail store for the menu's grid layout. A
// wallpaper directory can hold hundreds of multi-megabyte photos, so decoding them on the
// render path would freeze the overlay. Instead Get returns a cached thumbnail if it is ready
// and otherwise schedules a bounded background decode and returns nil (the caller draws a
// placeholder tile); the menu's frame loop polls Pending/TakeDirty and redraws as thumbnails
// land. Decoding is pure Go (internal/imgutil), so the whole thing stays cgo-free and static.
package thumbs

import (
	"image"
	"sync"

	"github.com/crispuscrew/zinc/menu/internal/imgutil"
)

const (
	// maxThumbBytes caps how much of a source file is read. Wallpapers are large, so this is
	// far more generous than the icon cap, but still bounds a hostile or corrupt file.
	maxThumbBytes = 64 << 20
	// maxThumbPixels rejects a source whose declared dimensions would allocate too much when
	// decoded (up to ~8K fits; a crafted file claiming more is refused rather than OOM-ing).
	maxThumbPixels = 64 << 20
	// maxInFlight bounds concurrent decodes so a big grid cannot spawn hundreds of goroutines
	// each holding a full decoded image; a few at a time keeps peak memory modest.
	maxInFlight = 3
)

// Store decodes and caches thumbnails scaled to fit a fixed box, off the render path. It is
// safe for concurrent use: the render goroutine calls Get/Pending/TakeDirty while background
// goroutines decode.
type Store struct {
	boxW, boxH int

	mu       sync.Mutex
	cache    map[string]*image.RGBA // path -> thumbnail; a present nil means "decoded, no image"
	inflight map[string]bool        // paths currently decoding, so each is scheduled once
	dirty    bool                   // a decode has completed since the last TakeDirty

	sem chan struct{} // bounds concurrent decodes to cap(sem)
}

// New returns a store that fits thumbnails into boxW x boxH pixels.
func New(boxW, boxH int) *Store {
	return &Store{
		boxW:     boxW,
		boxH:     boxH,
		cache:    map[string]*image.RGBA{},
		inflight: map[string]bool{},
		sem:      make(chan struct{}, maxInFlight),
	}
}

// Get returns the thumbnail for path when it is decoded, or nil when it is not yet ready. On
// the first call for a path it schedules a background decode; a path that failed to decode
// caches nil and is never rescheduled (Get keeps returning nil for it). The bool reports
// whether the decode has been attempted (true even when the result is nil), which callers can
// use to distinguish "still loading" from "loaded, but no image".
func (store *Store) Get(path string) (*image.RGBA, bool) {
	if path == "" || store == nil {
		return nil, true
	}
	store.mu.Lock()
	if thumb, done := store.cache[path]; done {
		store.mu.Unlock()
		return thumb, true
	}
	if store.inflight[path] {
		store.mu.Unlock()
		return nil, false
	}
	store.inflight[path] = true
	store.mu.Unlock()

	go store.decode(path)
	return nil, false
}

// decode reads, scales, and caches one thumbnail, blocking on the concurrency semaphore first
// so at most maxInFlight run at once.
func (store *Store) decode(path string) {
	store.sem <- struct{}{}
	defer func() { <-store.sem }()

	thumb := imgutil.Fit(imgutil.Decode(path, maxThumbBytes, maxThumbPixels), store.boxW, store.boxH)

	store.mu.Lock()
	store.cache[path] = thumb // may be nil: records that we tried and there is nothing to draw
	delete(store.inflight, path)
	store.dirty = true
	store.mu.Unlock()
}

// Pending reports whether any decode is still in flight, so the caller knows to keep polling.
func (store *Store) Pending() bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.inflight) > 0
}

// TakeDirty reports whether any decode has completed since the previous call, clearing the
// flag. The caller redraws when it returns true so newly-decoded thumbnails appear.
func (store *Store) TakeDirty() bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	dirty := store.dirty
	store.dirty = false
	return dirty
}

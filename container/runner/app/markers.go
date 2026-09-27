package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func lockFile(path string, nonblock bool) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("attached: open %s: %w", path, err)
	}
	flags := syscall.LOCK_EX
	if nonblock {
		flags |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(file.Fd()), flags); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

type marker struct {
	file *os.File
	path string
}

func (mark *marker) release() { mark.file.Close(); os.Remove(mark.path) }

func claimMarker(directory string) (*marker, error) {
	file, err := os.CreateTemp(directory, "term.*")
	if err != nil {
		return nil, fmt.Errorf("attached: create marker: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, fmt.Errorf("attached: lock marker: %w", err)
	}
	return &marker{file: file, path: file.Name()}, nil
}

// Only a successfully acquired lock proves a marker is stale; uncertainty keeps the holder alive.
func anyLive(directory string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return true
	}
	live := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "term.") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			live = true
			continue
		}
		if syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			file.Close()
			os.Remove(path)
			continue
		}
		file.Close()
		live = true
	}
	return live
}

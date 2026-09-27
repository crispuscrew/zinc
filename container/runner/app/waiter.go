package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type waiter struct {
	runRoot     string
	background  bool
	ensureUp    func() error
	runTerminal func() error
	stop        func() error
}

func (wait *waiter) run(app string) error {
	directory := filepath.Join(wait.runRoot, app)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("attached: create %s: %w", directory, err)
	}
	coord, err := lockFile(filepath.Join(directory, "lock"), false)
	if err != nil {
		return err
	}
	if err := wait.ensureUp(); err != nil {
		coord.Close()
		return err
	}
	mark, err := claimMarker(directory)
	if err != nil {
		coord.Close()
		return err
	}
	coord.Close()
	runErr := wait.runTerminal()
	coord, err = lockFile(filepath.Join(directory, "lock"), false)
	if err != nil {
		mark.release()
		return errors.Join(runErr, err)
	}
	defer coord.Close()
	mark.release()
	if !wait.background && !anyLive(directory) {
		return errors.Join(runErr, wait.stop())
	}
	return runErr
}

func runRoot() (string, error) {
	if directory := os.Getenv("XDG_RUNTIME_DIR"); directory != "" {
		return filepath.Join(directory, "zinc", "run"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("attached: locate runtime dir: %w", err)
	}
	return filepath.Join(cache, "zinc", "run"), nil
}

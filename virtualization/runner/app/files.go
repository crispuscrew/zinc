package app

import (
	"fmt"
	"os"
)

// removeIfPresent deletes a path, treating "already gone" as success: a reset should not
// fail because an app never got as far as building its seed.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

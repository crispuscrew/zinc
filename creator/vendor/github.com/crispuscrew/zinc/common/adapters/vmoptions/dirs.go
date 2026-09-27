package vmoptions

import (
	"fmt"
	"os"
	"path/filepath"
)

func prepareDirs(root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	path := root
	for _, component := range []string{"zinc", "runtime", "vm"} {
		path = filepath.Join(path, component)
		if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("VM options directory %s: require a directory not writable by other users", path)
		}
	}
	return nil
}

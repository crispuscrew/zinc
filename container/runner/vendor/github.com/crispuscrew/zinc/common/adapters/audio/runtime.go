package audio

import (
	"fmt"
	"os"
	"syscall"
)

func privateRuntime(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("audio runtime directory: %w", err)
	}
	stat, valid := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !valid || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("audio: runtime directory must be a private directory owned by the broker user")
	}
	return nil
}

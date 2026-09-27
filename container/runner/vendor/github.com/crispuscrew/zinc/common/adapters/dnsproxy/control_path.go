package dnsproxy

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

type controlListener struct {
	listener *net.UnixListener
	path     string
	identity os.FileInfo
}

func trustedOwner(info os.FileInfo) bool {
	stat, valid := info.Sys().(*syscall.Stat_t)
	return valid && trustedUID(stat.Uid)
}

func trustedUID(uid uint32) bool {
	return uid == 0 || uid == uint32(os.Geteuid())
}

func validateControlPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("control socket requires a clean absolute path")
	}
	parent := filepath.Dir(path)
	for current := parent; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || !trustedOwner(info) {
			return fmt.Errorf("untrusted control directory %q", current)
		}
		permissions := info.Mode().Perm()
		if current == parent && permissions&0077 != 0 {
			return fmt.Errorf("control socket parent must be private (0700)")
		}
		if permissions&0022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("writable control ancestor %q", current)
		}
		if current == "/" {
			return nil
		}
	}
}

func socketIdentity(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0600 || !trustedOwner(info) {
		return nil, fmt.Errorf("control socket must be owner/root-owned with mode 0600")
	}
	return info, nil
}

func listenControl(path string) (*controlListener, error) {
	if err := validateControlPath(path); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("control socket path must not exist: %q", path)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	identity, err := os.Lstat(path)
	control := &controlListener{listener: listener, path: path, identity: identity}
	if err == nil {
		err = os.Chmod(path, 0600)
	}
	if err != nil {
		return nil, errors.Join(err, control.close())
	}
	return control, nil
}

func (control *controlListener) close() error {
	closeErr := control.listener.Close()
	if errors.Is(closeErr, net.ErrClosed) {
		closeErr = nil
	}
	current, err := os.Lstat(control.path)
	if errors.Is(err, os.ErrNotExist) {
		return closeErr
	}
	if err != nil {
		return errors.Join(closeErr, err)
	}
	if control.identity != nil && os.SameFile(control.identity, current) {
		return errors.Join(closeErr, os.Remove(control.path))
	}
	return closeErr
}

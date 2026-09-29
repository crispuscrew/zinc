// Package paths resolves existing persistent VM data and private runtime paths.
package paths

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

type Paths struct {
	StateDir string
	ImageDir string
	RunDir   string
}

func Default() (Paths, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	runDir := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(dataHome) || !filepath.IsAbs(runDir) {
		return Paths{}, fmt.Errorf("XDG data and runtime directories must be absolute; XDG_RUNTIME_DIR is required")
	}
	return Paths{filepath.Join(dataHome, "zinc", "vms"), filepath.Join(dataHome, "zinc", "images"), filepath.Join(runDir, "zinc", "vm")}, nil
}

func (paths Paths) Overlay(name string) string { return filepath.Join(paths.StateDir, name+".qcow2") }
func (paths Paths) Seed(name string) string    { return filepath.Join(paths.StateDir, name+"-seed.iso") }
func (paths Paths) Log(name string) string     { return filepath.Join(paths.StateDir, name+".log") }
func (paths Paths) UEFIVars(name string) string {
	return filepath.Join(paths.StateDir, name+"-uefi-vars.fd")
}
func (paths Paths) TPMState(name string) string { return filepath.Join(paths.StateDir, name+"-tpm") }
func (paths Paths) PIDFile(name string) string  { return filepath.Join(paths.RunDir, name+".pid") }
func (paths Paths) QMP(name string) string      { return filepath.Join(paths.RunDir, name+".qmp") }
func (paths Paths) Serial(name string) string   { return filepath.Join(paths.RunDir, name+".serial") }
func (paths Paths) Resolv(name string) string {
	return filepath.Join(paths.RunDir, name+".resolv.conf")
}
func (paths Paths) TPMSocket(name string) string { return filepath.Join(paths.RunDir, name+".tpm") }
func (paths Paths) TPMPID(name string) string    { return filepath.Join(paths.RunDir, name+".tpm.pid") }

func (paths Paths) Layout(name string, seeded bool) qemu.Layout {
	layout := qemu.Layout{Overlay: paths.Overlay(name), PIDFile: paths.PIDFile(name), QMP: paths.QMP(name), Serial: paths.Serial(name)}
	if seeded {
		layout.Seed = paths.Seed(name)
	}
	return layout
}

func (paths Paths) EnsureDirs() error {
	for _, path := range []string{paths.StateDir, paths.ImageDir, paths.RunDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("VM directory %s must be a real directory not writable by other users", path)
		}
		if path == paths.RunDir && info.Mode().Perm() != 0o700 {
			return fmt.Errorf("VM runtime directory %s must have mode 0700", path)
		}
	}
	return nil
}

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/firmware"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

func prepareInstall(cfg schema.AppConfig, runtime vmoptions.Config, layout qemu.Layout, resume bool) (func(), error) {
	paths, err := paths.Default()
	if err != nil {
		return nil, err
	}
	if err := paths.EnsureDirs(); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(paths.RunDir, cfg.AppNameID+".lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("another installer is using this disk")
	}
	cleanup := func() {
		firmware.StopTPM(paths.TPMSocket(cfg.AppNameID), paths.TPMPID(cfg.AppNameID))
		for _, path := range []string{layout.PIDFile, layout.QMP, layout.Serial} {
			_ = os.Remove(path)
		}
		lock.Close()
	}
	if err := installDisk(runtime.Image, runtime.DiskSizeGiB, resume); err != nil {
		lock.Close()
		return nil, err
	}
	if _, err := firmware.Prepare(cfg.StartConditions, runtime.Image+".uefi-vars.fd", ""); err != nil {
		lock.Close()
		return nil, err
	}
	if cfg.StartConditions.TPM {
		if _, err := firmware.StartTPM(runtime.Image+".tpm-state", layout.TPMSocket, paths.TPMPID(cfg.AppNameID)); err != nil {
			cleanup()
			return nil, err
		}
	}
	return cleanup, nil
}

func installDisk(path string, size int64, resume bool) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || !resume {
			return fmt.Errorf("existing installation target requires --resume and must be a regular file")
		}
		fmt.Fprintf(os.Stderr, "WARNING: THIS WILL WRITE TO EXISTING DISK %s. Restore a backup to roll back.\n", path)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".install-*.qcow2")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Close(); err != nil {
		return err
	}
	output, err := exec.Command("qemu-img", "create", "-f", "qcow2", file.Name(), strconv.FormatInt(size, 10)+"G").CombinedOutput()
	if err != nil {
		return fmt.Errorf("create installation disk: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Chmod(file.Name(), 0o600); err != nil {
		return err
	}
	return os.Link(file.Name(), path)
}

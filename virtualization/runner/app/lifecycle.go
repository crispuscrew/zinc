package app

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/firmware"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/machine"
)

func guestName(name string) error { return vmoptions.Name(name) }

// Lock files remain after use so concurrent processes always lock the same inode.
func (svc Service) lock(name string) (*os.File, error) {
	if err := guestName(name); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(svc.Paths.RunDir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(svc.Paths.RunDir, name+".lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("%s: another lifecycle operation is in progress", name)
	}
	return file, nil
}

func (svc Service) Stop(name string, force bool, timeout time.Duration) error {
	return svc.stop(name, force, timeout, 0)
}

func (svc Service) stop(name string, force bool, timeout time.Duration, expectedPID int) (failure error) {
	if err := guestName(name); err != nil {
		return err
	}
	if expectedPID == 0 {
		if err := svc.markStopped(name); err != nil {
			return err
		}
	}
	lock, err := svc.stopLock(name, timeout)
	if err != nil {
		return err
	}
	record, recordErr := svc.readSupervisor(name)
	stopIssued := false
	defer func() {
		lock.Close()
		if stopIssued && failure == nil && recordErr == nil && record.PID != os.Getpid() {
			failure = waitSupervisor(record)
		}
	}()
	if recordErr != nil && !os.IsNotExist(recordErr) {
		return recordErr
	}
	if expectedPID != 0 {
		state, err := svc.State(name)
		if err != nil {
			return err
		}
		if !state.Alive || state.PID != expectedPID {
			return nil
		}
	}
	if err := svc.markStopped(name); err != nil {
		return err
	}
	stopIssued = true
	if err := svc.Runtime.Stop(name, force, timeout); err != nil {
		return err
	}
	firmware.StopTPM(svc.Paths.TPMSocket(name), svc.Paths.TPMPID(name))
	return removeIfPresent(svc.Paths.Resolv(name))
}

func (svc Service) Reset(name string) error {
	lock, err := svc.lock(name)
	if err != nil {
		return err
	}
	defer lock.Close()
	if record, err := svc.readSupervisor(name); err == nil && machine.SameProcess(record.PID, record.Identity) {
		return fmt.Errorf("%s has a live supervisor; stop it before reset", name)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	state, err := svc.State(name)
	if err != nil {
		return err
	}
	if state.Alive {
		return fmt.Errorf("%s is running; stop it before resetting", name)
	}
	firmware.StopTPM(svc.Paths.TPMSocket(name), svc.Paths.TPMPID(name))
	for _, path := range []string{svc.Paths.Overlay(name), svc.Paths.Seed(name), svc.Paths.UEFIVars(name), svc.manifestPath(name)} {
		if err := removeIfPresent(path); err != nil {
			return err
		}
	}
	return os.RemoveAll(svc.Paths.TPMState(name))
}

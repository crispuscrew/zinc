package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/machine"
)

type supervisorRecord struct {
	PID                  int
	Identity, Generation string
}

func (svc Service) controlPath(name, suffix string) string {
	return filepath.Join(svc.Paths.RunDir, name+"."+suffix)
}

func (svc Service) readSupervisor(name string) (supervisorRecord, error) {
	var record supervisorRecord
	file, err := os.OpenFile(svc.controlPath(name, "supervisor.json"), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return record, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() {
		return record, fmt.Errorf("invalid supervisor state file")
	}
	if err := ipc.Decode(file, &record); err != nil {
		return record, err
	}
	if record.PID <= 1 || len(record.Generation) != 32 || record.Identity == "" {
		return record, fmt.Errorf("invalid supervisor identity")
	}
	return record, nil
}

func (svc Service) writeControl(name, suffix string, body []byte) error {
	file, err := os.CreateTemp(svc.Paths.RunDir, ".control-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(body)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), svc.controlPath(name, suffix))
}

func (svc Service) registerSupervisor(name, generation string) (func(), error) {
	identity, err := machine.ProcessIdentity(os.Getpid())
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(supervisorRecord{os.Getpid(), identity, generation})
	if err != nil {
		return nil, err
	}
	if err := svc.writeControl(name, "supervisor.json", body); err != nil {
		return nil, err
	}
	if err := removeIfPresent(svc.controlPath(name, "fault")); err != nil {
		return nil, err
	}
	return func() {
		current, err := svc.readSupervisor(name)
		if err == nil && current.Generation == generation {
			_ = os.Remove(svc.controlPath(name, "supervisor.json"))
			_ = os.Remove(svc.controlPath(name, "stop"))
		}
	}, nil
}

func (svc Service) stopRequested(name, generation string) bool {
	body, err := os.ReadFile(svc.controlPath(name, "stop"))
	if os.IsNotExist(err) {
		return false
	}
	return err != nil || strings.TrimSpace(string(body)) == generation
}

func (svc Service) markStopped(name string) error {
	record, err := svc.readSupervisor(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !machine.SameProcess(record.PID, record.Identity) {
		return nil
	}
	return svc.writeControl(name, "stop", []byte(record.Generation+"\n"))
}

func (svc Service) supervisorLock(name string) (*os.File, error) {
	file, err := os.OpenFile(svc.controlPath(name, "supervisor.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("%s already has a live supervisor", name)
	}
	return file, nil
}

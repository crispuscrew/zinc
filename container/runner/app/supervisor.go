package app

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/adapters/ipc"
)

const superviseCommand = "__supervise"

type supervisorReady struct {
	Ready bool
	Error string
}

func startSupervisor(cfg schema.AppConfig) (*ipc.Child, error) {
	child, err := ipc.Start([]string{superviseCommand, cfg.AppNameID}, cfg)
	if err != nil {
		return nil, fmt.Errorf("supervise %s: %w", cfg.AppNameID, err)
	}
	var ready supervisorReady
	err = ipc.Receive(child.Status, &ready, ipc.ReadyTimeout)
	if err == nil && ready.Error != "" {
		err = errors.New(ready.Error)
	}
	if err == nil && !ready.Ready {
		err = fmt.Errorf("supervisor did not acknowledge launch snapshot")
	}
	if err != nil {
		return nil, fmt.Errorf("supervise %s: %w", cfg.AppNameID, errors.Join(err, child.Close()))
	}
	return child, nil
}

// HoldSupervisor accepts the resolved launch snapshot, never a fresh store lookup.
// Readiness precedes activation so startup failure leaves cleanup with the launcher.
func (svc Service) HoldSupervisor(name string, wait func(string) error) (failure error) {
	status, request, activation, err := ipc.Inherited()
	if err != nil {
		return err
	}
	defer status.Close()
	defer request.Close()
	defer activation.Close()
	ready := false
	defer func() {
		if !ready && failure != nil {
			_ = ipc.Send(status, supervisorReady{Error: failure.Error()})
		}
	}()
	var cfg schema.AppConfig
	if err := ipc.Receive(request, &cfg, ipc.ReadyTimeout); err != nil {
		return err
	}
	request.Close()
	if cfg.AppNameID != name || cfg.Type != schema.ZincContainer {
		return fmt.Errorf("supervisor request identity mismatch")
	}
	if err := validate.Validate(cfg); err != nil {
		return err
	}
	if _, err := svc.runtime.Running(); err != nil {
		return fmt.Errorf("supervisor cannot observe containers: %w", err)
	}
	if err := ipc.Send(status, supervisorReady{Ready: true}); err != nil {
		return err
	}
	status.Close()
	ready = true
	if err := activation.SetReadDeadline(time.Now().Add(ipc.ReadyTimeout)); err != nil {
		return err
	}
	var acknowledgement [1]byte
	if _, err := io.ReadFull(activation, acknowledgement[:]); err != nil {
		return fmt.Errorf("supervisor launch cancelled: %w", err)
	}
	if acknowledgement[0] != 1 {
		return fmt.Errorf("invalid supervisor activation")
	}
	activation.Close()
	return svc.Supervise(cfg, wait)
}

// Supervise preserves the launch lock and live-relaunch guard before idempotent teardown.
func (svc Service) Supervise(cfg schema.AppConfig, wait func(name string) error) error {
	if err := wait(cfg.AppNameID); err != nil {
		return fmt.Errorf("supervise %s: %w", cfg.AppNameID, err)
	}
	lock := lockLaunch(cfg.AppNameID)
	defer lock.close()
	if svc.runtime.IsRunning(cfg.AppNameID) {
		return nil
	}
	if err := svc.Stop(cfg); err != nil {
		return fmt.Errorf("supervise %s: tear down after the app exited: %w", cfg.AppNameID, err)
	}
	return nil
}

package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
)

// DefaultPreparationTimeout covers the complete initial preparation response:
// DNS, base verification, dependencies, disks, TPM, audio and QEMU readiness.
// Individual IPC messages and the final acknowledgement retain their own bounds.
const DefaultPreparationTimeout = 10 * time.Minute

func (svc Service) preparationBudget() (time.Duration, error) {
	if svc.PreparationTimeout < 0 {
		return 0, fmt.Errorf("VM preparation timeout must not be negative")
	}
	if svc.PreparationTimeout == 0 {
		return DefaultPreparationTimeout, nil
	}
	return svc.PreparationTimeout, nil
}

func (svc Service) awaitSupervisor(child *ipc.Child) error {
	budget, err := svc.preparationBudget()
	var ready LaunchReady
	if err == nil {
		if receiveErr := ipc.ReceiveWithin(child.Status, &ready, budget); receiveErr != nil {
			err = fmt.Errorf("waiting for VM preparation (budget %s): %w", budget, receiveErr)
		}
	}
	if err == nil && ready.Error != "" {
		err = fmt.Errorf("VM startup: %s", ready.Error)
	}
	if err == nil && ready.PID <= 1 {
		err = fmt.Errorf("supervisor did not confirm a guest PID")
	}
	if err == nil {
		err = child.Commit()
	}
	if err != nil {
		return errors.Join(err, child.Close())
	}
	return nil
}

func confirmStartup(status, lifetime *os.File, pid int) error {
	if err := ipc.Send(status, LaunchReady{PID: pid}); err != nil {
		return err
	}
	status.Close()
	if err := lifetime.SetReadDeadline(time.Now().Add(ipc.ReadyTimeout)); err != nil {
		return err
	}
	var acknowledgement [1]byte
	if _, err := io.ReadFull(lifetime, acknowledgement[:]); err != nil {
		return fmt.Errorf("launcher disappeared before accepting guest: %w", err)
	}
	if acknowledgement[0] != 1 {
		return fmt.Errorf("invalid launcher acknowledgement")
	}
	lifetime.Close()
	return nil
}

func acceptStartup(guest execution, confirm func(int) error, stop func(int) error) error {
	if err := confirm(guest.Process.PID); err != nil {
		return errors.Join(err, stop(guest.Process.PID), guest.Cleanup())
	}
	return nil
}

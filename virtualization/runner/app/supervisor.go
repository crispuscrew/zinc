package app

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
)

func (svc Service) HoldSupervisor(name, generation string) (failure error) {
	status, input, lifetime, err := ipc.Inherited()
	if err != nil {
		return err
	}
	defer status.Close()
	defer input.Close()
	defer lifetime.Close()
	ready := false
	defer func() {
		if !ready && failure != nil {
			_ = ipc.Send(status, LaunchReady{Error: failure.Error()})
		}
	}()
	var request LaunchRequest
	if err := ipc.Receive(input, &request); err != nil {
		return err
	}
	input.Close()
	decoded, err := hex.DecodeString(generation)
	if err != nil || len(decoded) != 16 || request.Generation != generation || request.Config.AppNameID != name {
		return fmt.Errorf("supervisor request identity mismatch")
	}
	if request.Paths != svc.Paths || request.AudioRuntimeDir != svc.AudioRuntimeDir {
		return fmt.Errorf("supervisor paths differ from inherited environment")
	}
	svc.Options = request.Options
	if err := svc.check(request.Config); err != nil {
		return err
	}
	operation, err := svc.lock(name)
	if err != nil {
		return err
	}
	owner, err := svc.supervisorLock(name)
	if err != nil {
		operation.Close()
		return err
	}
	defer owner.Close()
	state, err := svc.Runtime.State(name)
	if err != nil || state.Alive {
		operation.Close()
		if err != nil {
			return err
		}
		return fmt.Errorf("%s is already running", name)
	}
	release, err := svc.registerSupervisor(name, generation)
	operation.Close()
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	confirm := func(pid int) error {
		if err := confirmStartup(status, lifetime, pid); err != nil {
			return err
		}
		ready = true
		return nil
	}
	return svc.supervise(ctx, request.Config, generation, confirm, os.Stderr)
}

func combineExit(exit, cleanup error) error {
	if exit != nil {
		return errors.Join(exit, cleanup)
	}
	return cleanup
}

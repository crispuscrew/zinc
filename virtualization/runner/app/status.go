package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/crispuscrew/zinc/virtualization/runner/adapters/machine"
)

func waitSupervisor(record supervisorRecord) error {
	deadline := time.Now().Add(20 * time.Second)
	for machine.SameProcess(record.PID, record.Identity) {
		if time.Now().After(deadline) {
			return fmt.Errorf("guest stopped but supervisor cleanup did not finish within 20s")
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil
}

func (svc Service) annotateState(state machine.State) (machine.State, error) {
	record, err := svc.readSupervisor(state.Name)
	if err != nil && !os.IsNotExist(err) {
		return state, err
	}
	if err == nil && !state.Alive && machine.SameProcess(record.PID, record.Identity) {
		state.Guest, state.Detail = "restarting", "supervisor active; guest is not currently running"
	}
	if body, err := os.ReadFile(svc.controlPath(state.Name, "fault")); err == nil {
		state.Detail = strings.TrimSpace(string(body))
	} else if !os.IsNotExist(err) {
		return state, err
	}
	return state, nil
}

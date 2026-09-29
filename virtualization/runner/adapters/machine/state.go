package machine

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/qmp"
)

func (runtime Runtime) State(name string) (State, error) {
	state := State{Name: name}
	if err := vmoptions.Name(name); err != nil {
		return state, err
	}
	pid, err := runtime.readPID(name)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.PID, state.Alive = pid, alive(pid) && isGuestProcess(pid, name)
	if !state.Alive {
		return state, nil
	}
	session, err := qmp.Dial(runtime.Paths.QMP(name))
	if err != nil {
		state.Detail = err.Error()
		return state, nil
	}
	defer session.Close()
	status, err := session.QueryStatus()
	if err != nil {
		state.Detail = err.Error()
		return state, nil
	}
	state.Guest = status.Status
	return state, nil
}

func (runtime Runtime) Running() ([]State, error) {
	entries, err := os.ReadDir(runtime.Paths.RunDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var states []State
	for _, entry := range entries {
		name, found := strings.CutSuffix(entry.Name(), ".pid")
		if !found || strings.HasSuffix(name, ".tpm") {
			continue
		}
		state, err := runtime.State(name)
		if err != nil {
			return nil, err
		}
		if state.Alive {
			states = append(states, state)
		}
	}
	return states, nil
}

func (runtime Runtime) ConsolePath(name string) string { return runtime.Paths.Serial(name) }

func (runtime Runtime) readPID(name string) (int, error) {
	file, err := os.OpenFile(runtime.Paths.PIDFile(name), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("guest pidfile is not a regular file")
	}
	var data [32]byte
	read, err := file.Read(data[:])
	if err != nil {
		return 0, fmt.Errorf("unreadable guest pidfile: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data[:read])))
	if err != nil || pid <= 1 {
		return 0, fmt.Errorf("unreadable host pid for %s; refusing to assume the guest is stopped", name)
	}
	return pid, nil
}

func (runtime Runtime) clean(name string) error {
	for _, path := range []string{runtime.Paths.PIDFile(name), runtime.Paths.QMP(name), runtime.Paths.Serial(name)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

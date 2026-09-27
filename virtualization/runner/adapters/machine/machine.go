// Package machine starts, identifies and stops detached QEMU processes.
package machine

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/crispuscrew/zinc/virtualization/runner/adapters/qmp"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
)

const (
	startGrace   = 15 * time.Second
	pollInterval = 50 * time.Millisecond
	termGrace    = 5 * time.Second
)

type Runtime struct{ Paths paths.Paths }

type State struct {
	Name   string
	PID    int
	Alive  bool
	Guest  string
	Detail string
}

type Process struct {
	PID  int
	Done <-chan error
}

func (runtime Runtime) Start(name string, args, extraEnv []string, stdin string) error {
	_, err := runtime.Launch(name, args, extraEnv, stdin)
	return err
}

// Launch transfers exit-status ownership to the persistent supervisor.
func (runtime Runtime) Launch(name string, args, extraEnv []string, stdin string) (*Process, error) {
	state, err := runtime.State(name)
	if err != nil {
		return nil, err
	}
	if state.Alive {
		return nil, fmt.Errorf("%s is already running (pid %d)", name, state.PID)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty VM command")
	}
	if err := runtime.clean(name); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(runtime.Paths.Log(name), os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "\n=== %s starting ===\n", name)
	command := exec.Command(args[0], args[1:]...)
	command.Env = append(os.Environ(), extraEnv...)
	command.Stdout, command.Stderr = logFile, logFile
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait(); close(exited) }()
	abandon := func(err error) (*Process, error) {
		terminate(command.Process.Pid)
		_ = runtime.clean(name)
		return nil, err
	}
	if err := runtime.confirmStarted(name, command.Process.Pid); err != nil {
		return abandon(err)
	}
	pid := guestPID(command.Process.Pid, name)
	if pid == 0 {
		return abandon(fmt.Errorf("cannot identify started guest %s; see %s", name, runtime.Paths.Log(name)))
	}
	if err := os.WriteFile(runtime.Paths.PIDFile(name), []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		return abandon(err)
	}
	return &Process{PID: pid, Done: exited}, nil
}

func (runtime Runtime) confirmStarted(name string, pid int) error {
	deadline := time.Now().Add(startGrace)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return fmt.Errorf("guest exited during startup:\n%s", runtime.logTail(name))
		}
		if _, err := os.Stat(runtime.Paths.PIDFile(name)); err == nil {
			session, err := qmp.Dial(runtime.Paths.QMP(name))
			if err == nil {
				_, err = session.QueryStatus()
				session.Close()
				if err == nil {
					return nil
				}
			}
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("guest %s did not expose a working QMP socket within %s; see %s", name, startGrace, runtime.Paths.Log(name))
}

func (runtime Runtime) logTail(name string) string {
	data, err := os.ReadFile(runtime.Paths.Log(name))
	if err != nil {
		return "(guest log unavailable)"
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > 10 {
		lines = lines[len(lines)-10:]
	}
	return strings.Join(lines, "\n")
}

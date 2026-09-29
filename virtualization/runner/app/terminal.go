package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func terminalCommand(cfg schema.AppConfig) ([]string, error) {
	if !cfg.StartConditions.Terminal {
		return nil, nil
	}
	spec := os.Getenv("ZINC_TERMINAL")
	if spec == "" {
		spec = os.Getenv("TERMINAL")
	}
	var args []string
	if strings.HasPrefix(strings.TrimSpace(spec), "[") {
		if err := json.Unmarshal([]byte(spec), &args); err != nil {
			return nil, fmt.Errorf("ZINC_TERMINAL argv: %w", err)
		}
	} else {
		args = strings.Fields(spec)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("Terminal requires ZINC_TERMINAL (terminal argv, optionally a JSON array)")
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return nil, fmt.Errorf("terminal argv contains NUL")
		}
	}
	if _, err := exec.LookPath(args[0]); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("socat"); err != nil {
		return nil, fmt.Errorf("VM terminal sessions require socat: %w", err)
	}
	return args, nil
}

func startTerminal(cfg schema.AppConfig, terminal []string) error {
	if len(terminal) == 0 {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	args := append(append([]string(nil), terminal[1:]...), executable, "__console-session", cfg.AppNameID)
	if cfg.StopConditions.Background {
		args = append(args, "--background")
	}
	command := exec.Command(terminal[0], args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start VM terminal: %w", err)
	}
	return command.Process.Release()
}

// ConsoleSession runs in the spawned terminal. Closing it is an intentional stop,
// except when Background explicitly retains the guest. Guest commands are not run.
func (svc Service) ConsoleSession(name string, background bool) error {
	state, err := svc.State(name)
	if err != nil {
		return err
	}
	if !state.Alive {
		return fmt.Errorf("%s is not running", name)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	command := exec.CommandContext(ctx, "socat", "-,raw,echo=0", "unix-connect:"+svc.Paths.Serial(name))
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	result := command.Run()
	if background {
		return result
	}
	// The launching process holds its lifecycle lock until the terminal starts.
	for attempts := 0; attempts < 20; attempts++ {
		err = svc.stop(name, false, DefaultStopTimeout, state.PID)
		if err == nil || !strings.Contains(err.Error(), "operation is in progress") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.Join(result, err)
}

package ipc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type Child struct {
	Status     *os.File
	activation *os.File
	process    *os.Process
	done       chan struct{}
	result     error
}

func Start(arguments []string, request any) (*Child, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	var files []*os.File
	keep := false
	defer func() {
		if !keep {
			for _, file := range files {
				file.Close()
			}
		}
	}()
	for range 3 {
		reader, writer, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		files = append(files, reader, writer)
	}
	command := exec.Command(executable, arguments...)
	command.ExtraFiles = []*os.File{files[1], files[2], files[4]}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	child := &Child{Status: files[0], activation: files[5], process: command.Process, done: make(chan struct{})}
	go func() { child.result = command.Wait(); close(child.done) }()
	files[1].Close()
	files[2].Close()
	files[4].Close()
	if err := Send(files[3], request); err != nil {
		files[3].Close()
		return nil, errors.Join(err, child.Close())
	}
	files[3].Close()
	keep = true
	return child, nil
}

// Commit lets the acknowledged helper take ownership after the app has been started.
func (child *Child) Commit() error {
	if err := child.activation.SetWriteDeadline(time.Now().Add(ReadyTimeout)); err != nil {
		return err
	}
	_, err := child.activation.Write([]byte{1})
	return errors.Join(err, child.activation.Close(), child.Status.Close())
}

// Close cancels only this helper. The launcher retains cleanup ownership on failure.
func (child *Child) Close() error {
	child.activation.Close()
	child.Status.Close()
	select {
	case <-child.done:
		return child.result
	default:
	}
	if err := child.process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-child.done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("supervisor did not exit after cancellation")
	}
}

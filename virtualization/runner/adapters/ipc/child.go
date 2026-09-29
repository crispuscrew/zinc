package ipc

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type Child struct {
	Status  *os.File
	Control *os.File
	Process *os.Process
	Done    chan struct{}
	Grace   time.Duration
	result  error
	once    sync.Once
}

func Start(arguments []string, request any, diagnostic io.Writer) (*Child, error) {
	self, err := os.Executable()
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
	command := exec.Command(self, arguments...)
	command.ExtraFiles = []*os.File{files[1], files[2], files[4]}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Stderr = diagnostic
	if err := command.Start(); err != nil {
		return nil, err
	}
	child := &Child{Status: files[0], Control: files[5], Process: command.Process, Done: make(chan struct{})}
	go func() { child.result = command.Wait(); close(child.Done) }()
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

func (child *Child) Err() error {
	select {
	case <-child.Done:
		return child.result
	default:
		return nil
	}
}

// Close first closes the lifeline, allowing the helper to revoke gracefully.
// A helper that cannot shut down is terminated; this never targets the guest.
func (child *Child) Close() error {
	child.once.Do(func() { child.Control.Close(); child.Status.Close() })
	grace := child.Grace
	if grace == 0 {
		grace = 5 * time.Second
	}
	for _, signal := range []os.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		select {
		case <-child.Done:
			return child.result
		case <-time.After(grace):
			if err := child.Process.Signal(signal); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return err
			}
		}
		grace = 5 * time.Second
	}
	select {
	case <-child.Done:
		return child.result
	case <-time.After(grace):
		return errors.New("helper did not exit after SIGKILL")
	}
}

func (child *Child) Commit() error {
	if err := child.Control.SetWriteDeadline(time.Now().Add(ReadyTimeout)); err != nil {
		return err
	}
	_, err := child.Control.Write([]byte{1})
	return errors.Join(err, child.Control.Close(), child.Status.Close())
}

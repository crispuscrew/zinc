package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

const termStatusFD = 3
const termReadyTimeout = 30 * time.Second

func spawnTerminal(cfg schema.AppConfig, opt options.HostOptions, shell bool) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%s: locate self: %w", cfg.AppNameID, err)
	}
	data, err := json.Marshal(TerminalRequest{Config: cfg, Options: opt})
	if err != nil {
		return err
	}
	argv := []string{"__term", cfg.AppNameID}
	if shell {
		argv = append(argv, "--shell")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	process := exec.Command(executable, argv...)
	process.Stdin = bytes.NewReader(data)
	process.ExtraFiles = []*os.File{writer}
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := process.Start(); err != nil {
		writer.Close()
		return fmt.Errorf("%s: open terminal: %w", cfg.AppNameID, err)
	}
	go process.Wait()
	writer.Close()
	return readTermStatus(cfg.AppNameID, reader)
}

func readTermStatus(app string, pipe *os.File) error {
	if err := pipe.SetReadDeadline(time.Now().Add(termReadyTimeout)); err != nil {
		return err
	}
	line, err := bufio.NewReader(pipe).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("%s: the terminal reported nothing: %w", app, err)
	}
	line = strings.TrimSpace(line)
	if rest, ok := strings.CutPrefix(line, "error "); ok {
		return fmt.Errorf("%s: %s", app, rest)
	}
	if line != "ok" {
		return fmt.Errorf("%s: unreadable status from the terminal: %q", app, line)
	}
	return nil
}

func reportTerm(line string) {
	status := os.NewFile(termStatusFD, "status")
	if status == nil {
		return
	}
	fmt.Fprintln(status, strings.ReplaceAll(line, "\n", " "))
	status.Close()
}

package firmware

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type TPM struct {
	SocketPath string
	pidPath    string
}

func StartTPM(stateDir, socketPath, pidPath string) (*TPM, error) {
	if _, err := exec.LookPath("swtpm"); err != nil {
		return nil, fmt.Errorf("swtpm is required for StartConditions.TPM: %w", err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	command := exec.Command("swtpm", "socket", "--tpmstate", "dir="+stateDir,
		"--ctrl", "type=unixio,path="+socketPath, "--tpm2", "--flags", "startup-clear",
		"--daemon", "--pid", "file="+pidPath)
	if output, err := command.CombinedOutput(); err != nil {
		StopTPM(socketPath, pidPath)
		return nil, fmt.Errorf("start TPM: %w: %s", err, strings.TrimSpace(string(output)))
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(socketPath); err == nil && info.Mode()&os.ModeSocket != 0 {
			return &TPM{SocketPath: socketPath, pidPath: pidPath}, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	StopTPM(socketPath, pidPath)
	return nil, fmt.Errorf("TPM control socket did not appear within 3s")
}

func StopTPM(socketPath, pidPath string) {
	if data, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 && isSwtpm(pid) && ownsSocket(pid, socketPath) {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	_ = os.Remove(pidPath)
	_ = os.Remove(socketPath)
}

func ownsSocket(pid int, socket string) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	args := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	for index, value := range args {
		if value == "--ctrl" && index+1 < len(args) && args[index+1] == "type=unixio,path="+socket {
			return true
		}
	}
	return false
}

func isSwtpm(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	argv := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	return len(argv) > 0 && filepath.Base(argv[0]) == "swtpm"
}

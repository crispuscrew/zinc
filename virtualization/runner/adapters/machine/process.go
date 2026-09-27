package machine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func guestPID(started int, name string) int {
	if isGuestProcess(started, name) {
		return started
	}
	for _, child := range childrenOf(started) {
		if isGuestProcess(child, name) {
			return child
		}
	}
	return 0
}

func childrenOf(pid int) []int {
	name := strconv.Itoa(pid)
	data, err := os.ReadFile(filepath.Join("/proc", name, "task", name, "children"))
	if err != nil {
		return nil
	}
	var children []int
	for _, field := range strings.Fields(string(data)) {
		if child, err := strconv.Atoi(field); err == nil {
			children = append(children, child)
		}
	}
	return children
}

func terminate(started int) {
	for _, child := range childrenOf(started) {
		terminate(child)
	}
	if started > 1 {
		_ = syscall.Kill(started, syscall.SIGKILL)
	}
}

func alive(pid int) bool {
	if pid <= 1 || syscall.Kill(pid, 0) != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	closing := strings.LastIndexByte(string(data), ')')
	if closing < 0 {
		return false
	}
	fields := strings.Fields(string(data)[closing+1:])
	return len(fields) > 0 && fields[0] != "Z"
}

func isGuestProcess(pid int, name string) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	argv := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	if len(argv) == 0 || filepath.Base(argv[0]) != "qemu-system-x86_64" {
		return false
	}
	for index, value := range argv {
		if value == "-name" && index+1 < len(argv) && argv[index+1] == name {
			return true
		}
	}
	return false
}

func waitGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return true
		}
		time.Sleep(pollInterval)
	}
	return !alive(pid)
}

func isGone(err error) bool { return err == syscall.ESRCH }

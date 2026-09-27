package machine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcessIdentity pins the host PID and Linux start-time ticks against PID reuse.
func ProcessIdentity(pid int) (string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	closing := strings.LastIndexByte(string(data), ')')
	if closing < 0 {
		return "", fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(string(data)[closing+1:])
	if len(fields) < 20 || fields[0] == "Z" {
		return "", fmt.Errorf("process exited or has invalid stat")
	}
	return strconv.Itoa(pid) + ":" + fields[19], nil
}

func SameProcess(pid int, identity string) bool {
	current, err := ProcessIdentity(pid)
	return err == nil && current == identity
}

func (runtime Runtime) Cleanup(name string) error { return runtime.clean(name) }

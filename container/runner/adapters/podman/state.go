package podman

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func (Runtime) Exists(name string) bool {
	return exec.Command("podman", "container", "exists", name).Run() == nil
}

func (Runtime) IsRunning(name string) bool {
	running, err := isRunning(name)
	return err == nil && running
}

func isRunning(name string) (bool, error) {
	output, err := exec.Command("podman", "ps", "--filter", "name=^"+regexp.QuoteMeta(name)+"$", "--format", "{{.Names}}").Output()
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == name {
			return true, nil
		}
	}
	return false, nil
}

func (Runtime) Running() (map[string]bool, error) {
	names := map[string]bool{}
	output, err := exec.Command("podman", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, fmt.Errorf("list running containers: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			names[line] = true
		}
	}
	return names, nil
}

func (Runtime) PodOf(name string) (string, error) {
	output, err := exec.Command("podman", "inspect", "--format", "{{.Pod}}", name).Output()
	if err != nil {
		return "", fmt.Errorf("read the pod of %s: %w", name, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (Runtime) PIDs() (map[string]int, error) {
	output, err := exec.Command("podman", "ps", "--format", "{{.Names}} {{.Pid}}").Output()
	if err != nil {
		return nil, fmt.Errorf("list running container pids: %w", err)
	}
	return parsePIDs(string(output)), nil
}

func parsePIDs(output string) map[string]int {
	pids := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		name, text, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found || name == "" {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || pid <= 0 {
			continue
		}
		pids[name] = pid
	}
	return pids
}

func (Runtime) Logs(name string, tail int) (string, error) {
	output, err := exec.Command("podman", "logs", "--tail", strconv.Itoa(tail), name).CombinedOutput()
	return string(output), err
}

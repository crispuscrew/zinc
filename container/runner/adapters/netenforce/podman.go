package netenforce

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func podmanOutput(arguments ...string) (string, error) {
	const probeTimeout = 10 * time.Second
	probeContext, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	process := exec.CommandContext(probeContext, "podman", arguments...)
	var stderr bytes.Buffer
	process.Stderr = &stderr
	output, err := process.Output()
	if err != nil {
		return "", fmt.Errorf("podman %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(output), nil
}

func containerProcess(name string) (int, error) {
	output, err := podmanOutput("inspect", "--format", "{{.State.Pid}}", "--", name)
	if err != nil {
		return 0, fmt.Errorf("inspect network owner: %w", err)
	}
	process, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil || process <= 0 {
		return 0, fmt.Errorf("invalid running container process %q", output)
	}
	return process, nil
}

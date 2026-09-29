package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runtime binaries are resolved from $PATH.
const (
	Binary   = "zcr"
	VMBinary = "zvr"
)

// capture preserves failure diagnostics for the UI and forwards successful stderr
// (including RunnerFlags warnings) instead of discarding it after a successful launch.
func capture(binary string, args ...string) (string, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("%s not found on $PATH: install the Zinc runtime to launch apps (the picker still works without it)", binary)
	}
	var stdout, stderr bytes.Buffer
	command := exec.Command(path, args...)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return "", fmt.Errorf("%s", message)
		}
		if message := strings.TrimSpace(stdout.String()); message != "" {
			return "", fmt.Errorf("%s", message)
		}
		return "", fmt.Errorf("%s %s: %w", binary, strings.Join(args, " "), err)
	}
	if _, err := stderr.WriteTo(os.Stderr); err != nil {
		return "", fmt.Errorf("%s completed, but forwarding its diagnostics failed: %w", binary, err)
	}
	return stdout.String(), nil
}

package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func findBinary(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found on $PATH: install the Zinc runtime to run apps (authoring still works without it)", name)
	}
	return path, nil
}

func Passthrough(args ...string) error { return PassthroughTo(Binary, args...) }

func PassthroughTo(binary string, args ...string) error {
	path, err := findBinary(binary)
	if err != nil {
		return err
	}
	command := exec.Command(path, args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func capture(args ...string) (string, error) {
	stdout, _, err := CaptureTo(Binary, args...)
	return stdout, err
}

// Keep successful stderr separate so UI callers can show advisories without
// corrupting machine-readable stdout.
func CaptureTo(binary string, args ...string) (string, string, error) {
	path, err := findBinary(binary)
	if err != nil {
		return "", "", err
	}
	var stdout, stderr bytes.Buffer
	command := exec.Command(path, args...)
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return "", "", fmt.Errorf("%s", message)
		}
		if message := strings.TrimSpace(stdout.String()); message != "" {
			return "", "", fmt.Errorf("%s", message)
		}
		return "", "", fmt.Errorf("%s %s: %w", binary, strings.Join(args, " "), err)
	}
	return stdout.String(), strings.TrimSpace(stderr.String()), nil
}

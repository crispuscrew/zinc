package podman

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/crispuscrew/zinc/container/runner/ports"
)

func StopArgs(name string) []string    { return []string{"stop", name} }
func RestartArgs(name string) []string { return []string{"restart", name} }
func InspectArgs(name string) []string { return []string{"inspect", name} }
func LogsArgs(name string, follow bool) []string {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	return append(args, name)
}

func (Runtime) Exec(command ports.Command) error {
	process := exec.Command("podman", command.Args...)
	if command.Stdin != "" {
		process.Stdin = strings.NewReader(command.Stdin)
	}
	if output, err := process.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s%s", command.Desc, err, strings.TrimSpace(string(output)), helperImageHint(command, output))
	}
	return nil
}

func (Runtime) Capture(command ports.Command) (string, error) {
	process := exec.Command("podman", command.Args...)
	if command.Stdin != "" {
		process.Stdin = strings.NewReader(command.Stdin)
	}
	var stderr bytes.Buffer
	process.Stderr = &stderr
	output, err := process.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s%s", command.Desc, err, strings.TrimSpace(stderr.String()), helperImageHint(command, stderr.Bytes()))
	}
	return string(output), nil
}

func helperImageHint(command ports.Command, output []byte) string {
	if !strings.Contains(string(output), "image not known") {
		return ""
	}
	for _, arg := range command.Args {
		if strings.HasPrefix(arg, "zinc/") || strings.HasPrefix(arg, "localhost/zinc/") {
			return "\n  hint: " + arg + " is Zinc's own helper image and is built locally, never pulled." +
				"\n        build it once with: make -C container/runner netfilter-image"
		}
	}
	return ""
}

func (Runtime) Do(args []string) error {
	process := exec.Command("podman", args...)
	process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
	return process.Run()
}

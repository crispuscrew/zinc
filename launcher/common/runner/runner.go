// Package runner shells out to the `zcr` binary - Zinc's container runtime - so the
// launcher (zlt) can run the apps it lists without importing the runner. This is the
// same split zcc uses: zlt picks an app file, zcr reads that same file and runs it;
// they meet only at the on-disk format and this process boundary.
//
// zcr is expected on $PATH (installed alongside zlt). If it is missing, launching fails
// with an actionable message, while listing/filtering still works - those need only the
// shared library, not the runtime.
package runner

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Binary is the runtime binary the launcher delegates to; it is resolved from $PATH.
const Binary = "zcr"

// safeName guards the app argument at the exec boundary, independent of how zcr parses
// its arguments: a name that is empty or begins with '-' is rejected, so a filename- or
// CLI-derived token can never land in `zcr run`'s slot as a flag instead of an app. It
// deliberately allows '/', so `zlt <path>` still reaches zcr's path form.
func safeName(name string) error {
	if name == "" {
		return fmt.Errorf("empty app name")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("app name %q cannot begin with '-'", name)
	}
	return nil
}

// find locates the zcr binary, returning an actionable error if it is not installed.
func find() (string, error) {
	path, err := exec.LookPath(Binary)
	if err != nil {
		return "", fmt.Errorf("%s not found on $PATH: install the Zinc runtime to launch apps (the picker still works without it)", Binary)
	}
	return path, nil
}

// capture runs `zcr <args...>` and returns its stdout, folding stderr into the error so
// a caller can surface what went wrong.
func capture(args ...string) (string, error) {
	path, err := find()
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(path, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		if msg := strings.TrimSpace(stdout.String()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", fmt.Errorf("%s %s: %w", Binary, strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}

// Launch starts the app detached: `zcr run <name> --exec` (zcr validates, builds the
// derived image if needed, auto-starts dependencies, locks down the network, then
// detaches). zcr returns once the app is spawned.
func Launch(name string) error {
	if err := safeName(name); err != nil {
		return err
	}
	_, err := capture("run", name, "--exec")
	return err
}

// Stop tears the app's pod down: `zcr stop <name>`.
func Stop(name string) error {
	if err := safeName(name); err != nil {
		return err
	}
	_, err := capture("stop", name)
	return err
}

// Running returns the set of apps podman reports as up: `zcr ps`, one name per line. It
// is best-effort context for the picker (a running indicator), so a caller may ignore
// the error when zcr is absent.
func Running() (map[string]bool, error) {
	out, err := capture("ps")
	if err != nil {
		return nil, err
	}
	running := map[string]bool{}
	scan := bufio.NewScanner(strings.NewReader(out))
	for scan.Scan() {
		if name := strings.TrimSpace(scan.Text()); name != "" {
			running[name] = true
		}
	}
	return running, scan.Err()
}

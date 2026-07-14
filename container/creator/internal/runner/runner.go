// Package runner shells out to the `zcr` binary — Zinc's container runtime — so the
// creator (zcc) can run and manage the apps it authors without importing the runner.
// This is the whole zcc/zcr split: zcc writes app files and knows nothing about podman;
// zcr reads those same files and runs them. They meet only at the on-disk format and at
// this process boundary.
//
// zcr is expected on $PATH (it is installed alongside zcc). If it is missing, every
// runtime action here fails with an actionable message, while authoring (new/edit/
// validate/list) keeps working — those need only the shared library, not the runtime.
package runner

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Binary is the runtime binary the creator delegates to; it is resolved from $PATH.
const Binary = "zcr"

// Result is one image-search hit (name + registry description), mirroring what
// `zcr image search` prints, one tab-separated pair per line.
type Result struct {
	Name        string
	Description string
}

// find locates the zcr binary, returning an actionable error if it is not installed.
func find() (string, error) {
	path, err := exec.LookPath(Binary)
	if err != nil {
		return "", fmt.Errorf("%s not found on $PATH: install the Zinc runtime to run apps (authoring still works without it)", Binary)
	}
	return path, nil
}

// Passthrough runs `zcr <args...>` wired to the caller's own stdio and returns zcr's
// exit error verbatim. It is the CLI forwarder: `zcc run X` becomes `zcr run X`,
// streaming output live (follow logs, inspect JSON, the launch plan) and preserving the
// exit status, so `zcc` is a thin front-end over the runtime for those commands.
func Passthrough(args ...string) error {
	path, err := find()
	if err != nil {
		return err
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// capture runs `zcr <args...>` and returns its stdout. On failure it folds zcr's stderr
// into the error so a caller (the TUI) can surface what went wrong. Used for the
// programmatic actions that need the output as data rather than streamed to a terminal.
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

// Launch starts the app detached: `zcr run <name> --exec` (validate -> build derived
// image -> lock down egress -> detach). zcr returns once the app is spawned.
func Launch(name string) error {
	_, err := capture("run", name, "--exec")
	return err
}

// Stop tears the app's pod down: `zcr stop <name>`.
func Stop(name string) error {
	_, err := capture("stop", name)
	return err
}

// Plan returns the launch plan without running anything: `zcr run <name>` (no --exec
// prints the exact podman command(s) plus any nft ruleset that would be enforced).
func Plan(name string) (string, error) {
	return capture("run", name)
}

// Build (re)builds the app's derived image and returns zcr's build output.
func Build(name string) (string, error) {
	return capture("build", name)
}

// OpenTerminal opens one more terminal for a multiterminal app (`zcr term <name>`,
// `--shell` for a shell). zcr spawns a detached waiter and returns.
func OpenTerminal(name string, shell bool) error {
	args := []string{"term", name}
	if shell {
		args = append(args, "--shell")
	}
	_, err := capture(args...)
	return err
}

// Logs returns a snapshot of the app's logs: `zcr logs <name>` (no follow — it prints
// what podman has and exits).
func Logs(name string) (string, error) {
	return capture("logs", name)
}

// Resolve pins an image reference to its digest form: `zcr image resolve <ref>`.
func Resolve(ref string) (string, error) {
	out, err := capture("image", "resolve", ref)
	return strings.TrimSpace(out), err
}

// Search finds images by term: `zcr image search <term>`, parsing the name<TAB>desc
// lines it prints. An empty result is not an error.
func Search(term string) ([]Result, error) {
	out, err := capture("image", "search", term)
	if err != nil {
		return nil, err
	}
	var results []Result
	scan := bufio.NewScanner(strings.NewReader(out))
	for scan.Scan() {
		line := scan.Text()
		if strings.TrimSpace(line) == "" || line == "no images found" {
			continue
		}
		name, desc, _ := strings.Cut(line, "\t")
		results = append(results, Result{Name: name, Description: desc})
	}
	return results, scan.Err()
}

// Running returns the set of apps podman reports as up: `zcr ps`, one name per line.
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

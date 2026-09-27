//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

type environment struct {
	home, runtime, zc, zvr, zcr string
	variables                   []string
}

func newEnvironment(t *testing.T) environment {
	t.Helper()
	for _, binary := range []string{"qemu-system-x86_64", "qemu-img", "xorriso", "nsenter", "nft", "ip", "setpriv", "ssh", "ssh-keygen"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s unavailable", binary)
		}
	}
	kvm, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		t.Skip("accessible /dev/kvm required")
	}
	kvm.Close()
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	env := environment{home: t.TempDir()}
	env.runtime, err = os.MkdirTemp("", "zvm-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(env.runtime) })
	for _, item := range []struct {
		variable, module, binary string
		target                   *string
	}{
		{"ZINC_E2E_ZC", "../../creator", "zc", &env.zc},
		{"ZINC_E2E_ZVR", "../runner", "zvr", &env.zvr},
		{"ZINC_E2E_ZCR", "../../container/runner", "zcr", &env.zcr},
	} {
		if override := os.Getenv(item.variable); override != "" {
			*item.target = override
			continue
		}
		module := filepath.Join(here, item.module)
		command := exec.Command("make", "-C", module, "build")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %s %v", module, output, err)
		}
		*item.target = filepath.Join(module, "bin", item.binary)
	}
	env.variables = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(env.home, "config"),
		"XDG_DATA_HOME="+filepath.Join(env.home, "data"), "XDG_RUNTIME_DIR="+env.runtime,
		"PATH="+filepath.Dir(env.zvr)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return env
}

func (env environment) run(binary string, args ...string) (string, error) {
	command := exec.Command(binary, args...)
	command.Env = env.variables
	output, err := command.CombinedOutput()
	return string(output), err
}

func (env environment) must(t *testing.T, binary string, args ...string) string {
	t.Helper()
	output, err := env.run(binary, args...)
	if err != nil {
		t.Fatalf("%s %v: %s %v", binary, args, output, err)
	}
	return output
}

func (env environment) author(t *testing.T, name, base, digest string, port int, key string) {
	t.Helper()
	t.Cleanup(func() { _, _ = env.run(env.zvr, "stop", name, "--force") })
	inbound := `{"From":{"Type":"Host"},"To":{"Type":"Self","Filter":{"Ports":[22]}},"Protocols":["TCP"]}`
	env.must(t, env.zc, "new", name, "--vm", "--image", base, "--base-digest", digest,
		"--memory", "512", "--vcpus", "2", "--disk", "1", "--display", "None", "--loader-bios",
		"--disable-gpu", "--cloud-init", "--ci-ssh-key", key+".pub", "--interface", "primary",
		"--network-rule", inbound, "--forward", fmt.Sprintf("%d:22", port))
}

func (env environment) overlay(name string) string {
	return filepath.Join(env.home, "data", "zinc", "vms", name+".qcow2")
}

func (env environment) makeKey(t *testing.T) string {
	t.Helper()
	path := filepath.Join(env.home, "guest-key")
	env.must(t, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path)
	return path
}

func (env environment) ssh(t *testing.T, port int, key, command string) string {
	t.Helper()
	return env.must(t, "ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile="+filepath.Join(env.home, "known-hosts"), "-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=10", "-i", key, "-p", strconv.Itoa(port), "cirros@127.0.0.1", command)
}

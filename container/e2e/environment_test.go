//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const appImage = "localhost/zinc/e2e-app:local"

type environment struct{ creator, runner, apps string }

func tool(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	return string(output), err
}
func must(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := tool(name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return output
}
func waitFor(condition func() bool) bool {
	for attempt := 0; attempt < 40; attempt++ {
		if condition() {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
func (env environment) running(name string) bool {
	output, err := tool(env.runner, "ps")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}
func (env environment) start(t *testing.T, name string) {
	t.Helper()
	must(t, env.creator, "run", name, "--exec")
	if !waitFor(func() bool { return env.running(name) }) {
		t.Fatalf("%s did not start", name)
	}
}
func (env environment) logs(t *testing.T, name, marker string) string {
	t.Helper()
	var output string
	if !waitFor(func() bool { output, _ = tool(env.creator, "logs", name); return strings.Contains(output, marker) }) {
		t.Fatalf("%s did not log %q: %s", name, marker, output)
	}
	return output
}

func setup(t *testing.T) environment {
	t.Helper()
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not found on PATH")
	}
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	creator, runner := filepath.Join(here, "..", "..", "creator"), filepath.Join(here, "..", "runner")
	env := environment{creator: os.Getenv("ZINC_E2E_CREATOR"), runner: os.Getenv("ZINC_E2E_RUNNER")}
	for _, build := range []struct {
		module string
		binary *string
		name   string
	}{{creator, &env.creator, "zc"}, {runner, &env.runner, "zcr"}} {
		if *build.binary == "" {
			must(t, "make", "-C", build.module, "build")
			*build.binary = filepath.Join(build.module, "bin", build.name)
		}
		if _, err := os.Stat(*build.binary); err != nil {
			t.Fatal(err)
		}
	}
	must(t, "make", "-C", runner, "netfilter-image")
	must(t, "podman", "build", "-t", appImage, here)
	names := []string{"sleeper", "producer", "consumer", "capped", "slowdep", "waiter", "scratch", "busapp", "authored", "attached"}
	for _, name := range names {
		for _, args := range [][]string{{"container", "exists", name}, {"pod", "exists", name + "-pod"}, {"container", "exists", "zinc-dbus-" + name}} {
			if _, err := tool("podman", args...); err == nil {
				t.Fatalf("existing test resource %s; refusing to overwrite it", name)
			}
		}
	}
	config := t.TempDir()
	env.apps = filepath.Join(config, "zinc", "apps")
	if err := os.MkdirAll(env.apps, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range names[:7] {
		data, err := os.ReadFile(filepath.Join(here, "apps", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(env.apps, name+".yaml"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("PATH", filepath.Dir(env.runner)+string(os.PathListSeparator)+filepath.Dir(env.creator)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Cleanup(func() {
		for _, name := range names {
			tool(env.runner, "stop", name)
		}
	})
	return env
}

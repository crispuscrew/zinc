//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (env environment) authoring(t *testing.T) {
	must(t, env.creator, "new", "authored", "--image", appImage)
	if _, err := os.Stat(filepath.Join(env.apps, "authored.yaml")); err != nil {
		t.Fatal(err)
	}
	must(t, env.creator, "validate", "authored")
}

func (env environment) lifecycle(t *testing.T) {
	env.start(t, "sleeper")
	env.logs(t, "sleeper", "sleeper up")
	must(t, env.creator, "stop", "sleeper")
	if !waitFor(func() bool { return !env.running("sleeper") }) {
		t.Fatal("sleeper did not stop")
	}
}

func (env environment) containment(t *testing.T) {
	env.start(t, "capped")
	defer tool(env.creator, "stop", "capped")
	output := env.logs(t, "capped", "capped up")
	// Swap is an explicit raw Podman option; ResourcesMeta has no swap field.
	for _, want := range []string{"UID=65534", "MEMORY_MAX=134217728", "SWAP_MAX=33554432", "PIDS_MAX=50"} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %s in %s", want, output)
		}
	}
}

func (env environment) scratch(t *testing.T) {
	env.start(t, "scratch")
	defer tool(env.creator, "stop", "scratch")
	output := env.logs(t, "scratch", "scratch up")
	for _, want := range []string{"SCRATCH_FS=tmpfs", "SCRATCH_MB=8", "WROTE_MB=8", "READONLY=refused", "nosuid", "nodev", "noexec"} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %s in %s", want, output)
		}
	}
}

func (env environment) dependencies(t *testing.T) {
	defer tool(env.creator, "stop", "waiter")
	defer tool(env.creator, "stop", "slowdep")
	env.start(t, "waiter")
	if !waitFor(func() bool { return env.running("slowdep") }) {
		t.Fatal("dependency was not started")
	}
	// The canonical schema starts dependencies; it does not install readiness probes.
	health := must(t, "podman", "inspect", "--format", "{{json .Config.Healthcheck}}", "slowdep")
	if strings.Contains(health, "/run/ready") {
		t.Fatalf("legacy readiness behavior survived: %s", health)
	}
}

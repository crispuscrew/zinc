//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVMEndToEnd(t *testing.T) {
	// This scenario needs packet-preserving provisioned TAPs and a guest image
	// with matching static addressing. It must never substitute a slirp network.
	base, expected := os.Getenv("ZINC_E2E_STATIC_IMAGE"), os.Getenv("ZINC_E2E_STATIC_DIGEST")
	name := os.Getenv("ZINC_E2E_NETWORK_APP")
	if base == "" || expected == "" || name == "" || os.Getenv("ZINC_NETWORK_MANIFEST_DIR") == "" {
		t.Skip("provisioned TAP/static-guest fixtures are unavailable")
	}
	if !strings.HasPrefix(name, "zinc-e2e-") {
		t.Fatal("network fixture app must use the zinc-e2e- prefix")
	}
	port, err := strconv.Atoi(os.Getenv("ZINC_E2E_SSH_PORT"))
	if err != nil || port < 1024 || port > 65535 {
		t.Fatal("set the explicitly provisioned ZINC_E2E_SSH_PORT")
	}
	env := newEnvironment(t)
	if fileDigest(t, base) != expected {
		t.Fatal("static guest fixture does not match its authorized pin")
	}
	key := env.makeKey(t)
	env.author(t, name, base, expected, port, key)
	t.Run("authoring_separates_options", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join(env.home, "config", "zinc", "apps", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, removed := range []string{"VirtualizationMeta:", "BaseDigest:", "DiskSizeGiB:", "ForwardPorts:"} {
			if strings.Contains(string(body), removed) {
				t.Errorf("removed option in app YAML: %s", removed)
			}
		}
		path := filepath.Join(env.home, "config", "zinc", "runtime", "vm", name+".json")
		body, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var options map[string]any
		if err := json.Unmarshal(body, &options); err != nil {
			t.Fatal(err)
		}
		if options["BaseDigest"] != expected || options["Image"] != base {
			t.Fatal(options)
		}
		env.must(t, env.zvr, "validate", name)
		if output, err := env.run(env.zcr, "validate", name); err == nil || !strings.Contains(output, "zvr") {
			t.Fatalf("wrong-runtime refusal: %s %v", output, err)
		}
	})
	t.Run("dry_run_creates_no_state", func(t *testing.T) {
		output := env.must(t, env.zvr, "run", name, "--dry-run")
		for _, want := range []string{"qemu-system-x86_64", "accel=kvm", "-sandbox", "tap,id="} {
			if !strings.Contains(output, want) {
				t.Errorf("missing %s", want)
			}
		}
		if _, err := os.Stat(filepath.Join(env.home, "data")); !os.IsNotExist(err) {
			t.Fatal("dry-run created durable state")
		}
		entries, err := os.ReadDir(env.runtime)
		if err != nil || len(entries) != 0 {
			t.Fatalf("dry-run wrote runtime files: %v %v", entries, err)
		}
	})
	t.Run("filtered_boot_identity_and_loopback", func(t *testing.T) {
		env.must(t, env.zvr, "run", name)
		if !waitForSSH(port, 120*time.Second) {
			t.Fatal("guest SSH banner did not arrive")
		}
		assertLoopbackOnly(t, port)
		output := env.ssh(t, port, key, "hostname")
		if strings.TrimSpace(output) != name {
			t.Fatalf("cloud-init hostname not applied: %q", output)
		}
		if got := fileDigest(t, base); got != expected {
			t.Fatal("base changed while guest ran")
		}
		env.must(t, env.zvr, "status", name)
	})
	t.Run("graceful_stop_and_reset", func(t *testing.T) {
		start := time.Now()
		output := env.must(t, env.zvr, "stop", name)
		if time.Since(start) > 45*time.Second || strings.Contains(output, "terminating") {
			t.Fatal("graceful shutdown failed")
		}
		assertNothingLeftBehind(t, name)
		if _, err := os.Stat(env.overlay(name)); err != nil {
			t.Fatal(err)
		}
		env.must(t, env.zvr, "reset", name, "--confirm")
		if _, err := os.Stat(env.overlay(name)); !os.IsNotExist(err) {
			t.Fatal("reset kept overlay")
		}
		if _, err := os.Stat(filepath.Join(env.home, "config", "zinc", "runtime", "vm", name+".json")); err != nil {
			t.Fatal("reset removed runtime options:", err)
		}
	})
}

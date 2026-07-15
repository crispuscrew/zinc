//go:build e2e

// Package e2e drives the real zcc and zcr binaries against rootless podman and asserts
// that an app actually runs and that the network lock-down is actually enforced - the
// guarantees unit tests cannot prove. It is black-box: everything goes through os/exec,
// nothing is imported from the tools under test.
//
// Run with `make e2e` (which sets the build tag and a generous timeout). Requires podman;
// the test skips if it is absent. The heavy lifting (build the binaries and helper images
// if missing) happens in setup, so the test is self-contained.
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	appImage = "localhost/zinc/e2e-app:local"
	nftImage = "zinc/netfilter:local"
)

// tool runs a command, returning combined output and any error. The whole harness is
// this one primitive - no shell, no quoting, real errors.
func tool(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// must runs a command and fails the test if it errors, surfacing the output.
func must(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := tool(name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func TestE2E(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not found on $PATH; skipping end-to-end tests")
	}

	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	creator := filepath.Join(here, "..", "creator")
	runner := filepath.Join(here, "..", "runner")
	zcc := filepath.Join(creator, "bin", "zcc")
	zcr := filepath.Join(runner, "bin", "zcr")

	// Build what's missing: the two binaries, the nft helper image, and the test app image.
	if _, err := os.Stat(zcc); err != nil {
		must(t, "make", "-C", creator, "build")
	}
	if _, err := os.Stat(zcr); err != nil {
		must(t, "make", "-C", runner, "build")
	}
	if _, err := tool("podman", "image", "exists", nftImage); err != nil {
		must(t, "make", "-C", runner, "netfilter-image")
	}
	must(t, "podman", "build", "-t", appImage, here)

	// Isolate the store and running state; zcc delegates runtime actions to zcr on $PATH.
	cfg := t.TempDir()
	apps := filepath.Join(cfg, "zinc", "apps")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sleeper", "producer", "consumer"} {
		data, err := os.ReadFile(filepath.Join(here, "apps", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(apps, name+".yaml"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("PATH", filepath.Join(runner, "bin")+string(os.PathListSeparator)+
		filepath.Join(creator, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))

	t.Cleanup(func() {
		for _, app := range []string{"consumer", "producer", "sleeper"} {
			tool(zcr, "stop", app)
			tool("podman", "pod", "rm", "-f", app+"-pod")
			tool("podman", "rm", "-f", app)
		}
		tool("podman", "network", "rm", "-f", "zinc-link-producer")
	})

	running := func(name string) bool {
		out, _ := tool(zcr, "ps")
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) == name {
				return true
			}
		}
		return false
	}
	// waitFor polls the predicate for up to 20s (podman pod bring-up + nft load is not
	// instant), returning whether it became true.
	waitFor := func(cond func() bool) bool {
		for i := 0; i < 40; i++ {
			if cond() {
				return true
			}
			time.Sleep(500 * time.Millisecond)
		}
		return false
	}

	t.Run("authoring", func(t *testing.T) {
		must(t, zcc, "new", "authored", "--image", appImage)
		if _, err := os.Stat(filepath.Join(apps, "authored.yaml")); err != nil {
			t.Fatal("zcc new should have written authored.yaml")
		}
		must(t, zcc, "validate", "authored")
	})

	t.Run("lifecycle", func(t *testing.T) {
		must(t, zcc, "run", "sleeper", "--exec")
		if !waitFor(func() bool { return running("sleeper") }) {
			t.Fatal("sleeper should be running after `zcc run --exec`")
		}
		out, _ := tool(zcc, "logs", "sleeper")
		if !strings.Contains(out, "sleeper up") {
			t.Fatalf("zcc logs should return the app's output, got:\n%s", out)
		}
		must(t, zcc, "stop", "sleeper")
		if !waitFor(func() bool { return !running("sleeper") }) {
			t.Fatal("sleeper should be stopped after `zcc stop`")
		}
	})

	t.Run("tier2_enforcement", func(t *testing.T) {
		must(t, zcc, "run", "producer", "--exec")
		if !waitFor(func() bool { return running("producer") }) {
			t.Fatal("producer should be running")
		}
		must(t, zcc, "run", "consumer", "--exec")
		waitFor(func() bool { return running("consumer") })

		// The consumer's entrypoint probes the producer and prints the verdict to its logs.
		var verdict string
		waitFor(func() bool {
			out, _ := tool(zcc, "logs", "consumer")
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "PROBE ") {
					verdict = strings.TrimSpace(line)
				}
			}
			return verdict != ""
		})
		t.Logf("consumer reported: %q", verdict)

		if !strings.Contains(verdict, "5432=open") {
			t.Error("consumer should REACH the producer's published port 5432")
		}
		if !strings.Contains(verdict, "9999=closed") {
			t.Error("consumer should be DROPPED on the unpublished port 9999")
		}
	})
}

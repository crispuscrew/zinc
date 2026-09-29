//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A blank, read-only boot disk exercises real QEMU and supervision without an
// OS download, network namespace, published port or host audio connection.
func TestVMOfflineLifecycle(check *testing.T) {
	binary := os.Getenv("ZINC_E2E_ZVR")
	if binary == "" {
		check.Skip("set ZINC_E2E_ZVR to a freshly built canonical runner")
	}
	for _, tool := range []string{"qemu-system-x86_64", "qemu-img"} {
		if _, err := exec.LookPath(tool); err != nil {
			check.Skipf("%s unavailable", tool)
		}
	}
	acceleration := os.Getenv("ZINC_E2E_ACCEL")
	if acceleration != "tcg" {
		kvm, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
		if err != nil {
			check.Skip("accessible KVM is required, or explicitly select ZINC_E2E_ACCEL=tcg for the blank-disk smoke test")
		}
		kvm.Close()
	}
	root, err := os.MkdirTemp("", "zvm-offline-")
	if err != nil {
		check.Fatal(err)
	}
	env := environment{home: root, runtime: filepath.Join(root, "run"), zvr: binary}
	if err := os.Mkdir(env.runtime, 0o700); err != nil {
		check.Fatal(err)
	}
	env.variables = append(os.Environ(), "XDG_RUNTIME_DIR="+env.runtime, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_CONFIG_HOME="+filepath.Join(root, "config"))
	name := "offline-" + strings.TrimPrefix(filepath.Base(root), "zvm-offline-")
	check.Cleanup(func() {
		if output, err := env.run(binary, "stop", name, "--force"); err != nil {
			check.Logf("cleanup failed; retaining %s for inspection: %s %v", root, output, err)
			return
		}
		os.RemoveAll(root)
	})
	base := filepath.Join(root, "base.qcow2")
	env.must(check, "qemu-img", "create", "-f", "qcow2", base, "16M")
	pin := strings.TrimSpace(env.must(check, binary, "pin", base))
	config := map[string]any{"SchemaVersion": 4, "Type": "ZincVirtualization", "AppNameID": name,
		"ImageMeta": map[string]any{"Image": base}, "ResourcesMeta": map[string]any{"MaxRamMiB": 128, "MaxCPUCores": 1},
		"StartConditions": map[string]any{"LoaderBIOS": true, "ReadOnlyRootfs": true},
		"StopConditions":  map[string]any{"Autorestart": true}, "DisplayMeta": map[string]any{"DisableGpuAccess": true}}
	if acceleration == "tcg" {
		config["RunnerFlags"] = []string{"-machine", "q35,accel=tcg", "-cpu", "max"}
	}
	options := map[string]any{"Version": 1, "AppNameID": name, "Image": base, "BaseDigest": pin, "Display": "None", "Devices": "Virtio"}
	appPath, optionsPath := filepath.Join(root, "app.yaml"), filepath.Join(root, "options.json")
	writeJSON(check, appPath, config)
	writeJSON(check, optionsPath, options)
	plan := env.must(check, binary, "run", appPath, "--runtime-options", optionsPath, "--dry-run")
	if strings.Contains(plan, "-netdev") || strings.Contains(plan, "-audiodev") || strings.Contains(plan, "-snapshot") {
		check.Fatal(plan)
	}
	if _, err := os.Stat(filepath.Join(root, "data")); !os.IsNotExist(err) {
		check.Fatal("plan created VM state")
	}
	env.must(check, binary, "run", appPath, "--runtime-options", optionsPath)
	pid := readGuestPID(check, env, name)
	verifyGuestProcess(check, pid, env.overlay(name))
	assertReadOnlyBlock(check, filepath.Join(env.runtime, "zinc", "vm", name+".qmp"))
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		check.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	restarted := 0
	for time.Now().Before(deadline) {
		if current := guestPID(env, name); current > 1 && current != pid {
			restarted = current
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if restarted == 0 {
		check.Fatal("persistent supervisor did not restart a failed QEMU process")
	}
	verifyGuestProcess(check, restarted, env.overlay(name))
	env.must(check, binary, "stop", name, "--force")
	time.Sleep(3 * time.Second)
	if guestPID(env, name) != 0 {
		check.Fatal("manual stop was restarted")
	}
	if _, err := os.Stat(filepath.Join(env.runtime, "zinc", "vm", name+".supervisor.json")); !os.IsNotExist(err) {
		check.Fatal("supervisor state survived stop")
	}
	if fileDigest(check, base) != pin {
		check.Fatal("base image changed")
	}
	env.must(check, binary, "reset", name, "--confirm")
	if _, err := os.Stat(env.overlay(name)); !os.IsNotExist(err) {
		check.Fatal("reset left overlay")
	}
	config["AudioMeta"] = map[string]any{"Playback": map[string]any{"PipeWireDefault": true}}
	writeJSON(check, appPath, config)
	if output, err := env.run(binary, "run", appPath, "--runtime-options", optionsPath); err == nil || !strings.Contains(output, "audio") {
		check.Fatalf("missing private audio policy did not fail closed: %s %v", output, err)
	}
	if guestPID(env, name) != 0 {
		check.Fatal("guest started before audio readiness")
	}
	if _, err := os.Stat(filepath.Join(env.runtime, "zinc", "vm", name+".supervisor.json")); !os.IsNotExist(err) {
		check.Fatal("failed audio startup leaked supervisor state")
	}
	delete(config, "AudioMeta")
	writeJSON(check, appPath, config)
	env.must(check, binary, "reset", name, "--confirm")
	options["BaseDigest"] = "sha256:" + strings.Repeat("0", 64)
	writeJSON(check, optionsPath, options)
	if output, err := env.run(binary, "run", appPath, "--runtime-options", optionsPath); err == nil || !strings.Contains(output, "pinned digest") {
		check.Fatalf("pin refusal: %s %v", output, err)
	}
}

func writeJSON(check *testing.T, path string, value any) {
	check.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		check.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		check.Fatal(err)
	}
}

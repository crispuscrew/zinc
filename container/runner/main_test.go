package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsageAndUnknown(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := run(nil); err == nil || !strings.Contains(err.Error(), "usage: zcr") {
		t.Fatalf("no args should return usage, got: %v", err)
	}
	if err := run([]string{"bogus"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("a bogus command should be rejected, got: %v", err)
	}
}

func TestVersionDispatch(t *testing.T) {
	quiet(t)
	if err := run([]string{"version"}); err != nil {
		t.Fatalf("version: %v", err)
	}
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("--version: %v", err)
	}
}

func TestValidateDispatch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	good := writeApp(t, "SchemaVersion: 4\nType: ZincContainer\nAppNameID: demo\nImageMeta:\n  Image: docker.io/library/alpine"+digestPin+"\n")
	if err := run([]string{"validate", good}); err != nil {
		t.Fatalf("validate of a good app should pass, got: %v", err)
	}

	bad := writeApp(t, "SchemaVersion: 4\nType: ZincContainer\nAppNameID: demo\nImageMeta:\n  Image: alpine:latest\n")
	if err := run([]string{"validate", bad}); err == nil {
		t.Fatal("validate of a non-digest-pinned image should fail")
	}

	if err := run([]string{"validate"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("validate with no arg should return usage, got: %v", err)
	}
}

// Both app types share a store; a VM must be dispatched to zvr.
func TestVMAppRefusedByRunner(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	vm := writeApp(t, "SchemaVersion: 4\nType: ZincVirtualization\nAppNameID: guest\n"+
		"ImageMeta:\n  Image: /var/lib/zinc/images/fedora.qcow2\n"+
		"ResourcesMeta: {MaxRamMiB: 4096, MaxCPUCores: 2}\n")

	for _, command := range []string{"validate", "run", "stop", "inspect"} {
		err := run([]string{command, vm})
		if err == nil || !strings.Contains(err.Error(), "zvr") {
			t.Errorf("zcr %s on a VM app: want an error pointing at zvr, got: %v", command, err)
		}
	}
}

// run without --exec is a dry run: it validates and prints the podman plan, touching no
// runtime, so it succeeds for a valid app under test.
func TestRunDryRun(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	good := writeApp(t, "SchemaVersion: 4\nType: ZincContainer\nAppNameID: demo\nImageMeta:\n  Image: docker.io/library/alpine"+digestPin+"\n")
	if err := run([]string{"run", good}); err != nil {
		t.Fatalf("dry-run of a good app should succeed, got: %v", err)
	}
}

// Runtime-only volumes pass through the same validation and argv builder as authored ones.
func TestRunRuntimeVolumeInPlan(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	appPath := writeApp(t, "SchemaVersion: 4\nType: ZincContainer\nAppNameID: demo\nImageMeta:\n  Image: docker.io/library/alpine"+digestPin+"\n")
	var runErr error
	out := captureStdout(t, func() {
		runErr = run([]string{"run", appPath, "-v", "/host/dl:/downloads:rw", "--volume", "/etc/hosts:/etc/hosts"})
	})
	if runErr != nil {
		t.Fatalf("dry-run with runtime volumes should succeed, got: %v", runErr)
	}
	if !strings.Contains(out, "-v /host/dl:/downloads:rw,noexec") {
		t.Fatalf("writable runtime volume missing from plan:\n%s", out)
	}
	if !strings.Contains(out, "-v /etc/hosts:/etc/hosts:ro,noexec") {
		t.Fatalf("default read-only runtime volume missing from plan:\n%s", out)
	}
}

// A runtime volume whose spec would shift podman's -v fields (a ':' or whitespace in the
// host path) must be rejected, not silently mounted: it is appended to the config and the
// existing validation screens it before any arg is built.
func TestRunRuntimeVolumeRejectedByValidation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	appPath := writeApp(t, "SchemaVersion: 4\nType: ZincContainer\nAppNameID: demo\nImageMeta:\n  Image: docker.io/library/alpine"+digestPin+"\n")
	// A trailing ':' segment reads as an empty CONTAINER path (four fields would be
	// rejected at parse time); an unsafe char inside a field is caught at validation.
	if err := run([]string{"run", appPath, "-v", "/ho st:/inner"}); err == nil {
		t.Fatal("a host path with whitespace should be rejected by validation")
	}
}

// A file launch cannot impersonate a stored app's runtime and desktop identity.
func TestPathLoadedConfigCannotClaimAnotherAppsIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	quiet(t)

	apps := filepath.Join(home, "zinc", "apps")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "SchemaVersion: 4\nType: ZincContainer\nAppNameID: victim\nImageMeta:\n  Image: docker.io/library/alpine" + digestPin + "\n"
	stored := filepath.Join(apps, "victim.yaml")
	if err := os.WriteFile(stored, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// The same definition from somewhere else is a forgery, whatever it otherwise contains.
	err := run([]string{"validate", writeApp(t, body)})
	if err == nil {
		t.Fatal("a file claiming a defined app's AppNameID should be refused")
	}
	if !strings.Contains(err.Error(), "victim") {
		t.Errorf("the refusal should name what is being impersonated, got: %v", err)
	}

	// The store's own file, given by path, is not a forgery: that is the same app.
	if err := run([]string{"validate", stored}); err != nil {
		t.Fatalf("the store's own file should still load by path, got: %v", err)
	}

	// A name nothing in the store claims is nobody's identity to steal.
	free := strings.Replace(body, "AppNameID: victim", "AppNameID: unclaimed", 1)
	if err := run([]string{"validate", writeApp(t, free)}); err != nil {
		t.Fatalf("a file claiming an undefined name should load, got: %v", err)
	}
}

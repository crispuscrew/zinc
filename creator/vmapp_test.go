package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/store"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// zc authors both app types now, so `new --vm` has to write a config the VM runtime
// accepts: pinned base, real sizing, an explicit display mode.
func TestNewVM_WritesAValidVMApp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	err := run([]string{"new", "guest", "--vm",
		"--image", "/var/lib/zinc/images/fedora.qcow2",
		"--base-digest", testDigest,
		"--memory", "8192", "--vcpus", "4", "--disk", "40",
		"--display", "Accelerated"})
	if err != nil {
		t.Fatalf("new --vm: %v", err)
	}

	sto, err := store.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := sto.Load("guest")
	if err != nil {
		t.Fatalf("load the written app: %v", err)
	}
	if cfg.Type != schema.ZincVirtualization {
		t.Errorf("Type = %q, want %q", cfg.Type, schema.ZincVirtualization)
	}
	virt := cfg.VirtualizationMeta
	if virt.BaseDigest != testDigest || virt.MemoryMiB != 8192 || virt.VCPUs != 4 || virt.DiskSizeGiB != 40 {
		t.Errorf("VirtualizationMeta = %+v, want the flags as given", virt)
	}
	if virt.Display != schema.VMDisplayAccelerated {
		t.Errorf("Display = %q, want %q", virt.Display, schema.VMDisplayAccelerated)
	}
}

// A VM app without its pin must not save: the digest is what makes the base image the one
// that was authorised, and Save runs the same validation the runtime does.
func TestNewVM_RequiresThePin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	err := run([]string{"new", "guest", "--vm", "--image", "/var/lib/zinc/images/fedora.qcow2"})
	if err == nil || !strings.Contains(err.Error(), "BaseDigest") {
		t.Fatalf("a VM app with no base digest should be refused, got: %v", err)
	}
}

// VM flags on a container app would be written into a config where they do nothing, which
// is the trap the cross-type validation exists to close - so the CLI refuses them early
// with a message about the flag the author actually forgot.
func TestNew_VMFlagsWithoutVMRefused(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	err := run([]string{"new", "app", "--image", "localhost/app:local", "--memory", "2048"})
	if err == nil || !strings.Contains(err.Error(), "--vm") {
		t.Fatalf("VM flags without --vm should be refused, got: %v", err)
	}
}

// A container app still authors exactly as before.
func TestNew_ContainerUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	if err := run([]string{"new", "app", "--image", "localhost/app:local"}); err != nil {
		t.Fatalf("new (container): %v", err)
	}
	sto, _ := store.Default()
	cfg, err := sto.Load("app")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Type != schema.ZincContainer {
		t.Errorf("Type = %q, want %q", cfg.Type, schema.ZincContainer)
	}
	if !cfg.VirtualizationMeta.IsZero() {
		t.Errorf("a container app should carry no VM fields, got %+v", cfg.VirtualizationMeta)
	}
}

// Commands with no counterpart on the VM side are refused by name. Forwarding them to zvr
// would fail with an unknown-command error that says nothing about why, and silently
// doing nothing would be worse still.
func TestDelegateVM_UnsupportedCommandsExplainThemselves(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{"build", "no image to build"},
		{"logs", "no container log"},
		{"restart", "zc stop"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			err := delegateVM(tc.command, "guest", []string{"guest"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s on a VM app: want an error containing %q, got: %v", tc.command, tc.want, err)
			}
			if !strings.Contains(err.Error(), "VM app") {
				t.Errorf("the error should say the app is a VM, got: %v", err)
			}
		})
	}
}

// zc's own contract is that a bare `run` shows the plan and `--exec` performs it. zvr's
// default is the opposite, so the flag has to be translated rather than forwarded.
func TestDelegateVM_RunTranslatesTheExecContract(t *testing.T) {
	// The mapping is asserted through the argument helpers the translation is built from,
	// since running it would need a zvr binary on PATH.
	if got := firstPositional([]string{"--exec", "guest"}); got != "guest" {
		t.Errorf("firstPositional = %q, want the app name regardless of flag order", got)
	}
	if !hasFlag([]string{"guest", "--exec"}, "--exec") {
		t.Error("--exec should be recognised after the name")
	}
	if hasFlag([]string{"guest"}, "--exec") {
		t.Error("a bare run must not look like --exec")
	}
	if got := flagsOnly([]string{"guest", "--force"}); len(got) != 1 || got[0] != "--force" {
		t.Errorf("flagsOnly = %v, want just the flags", got)
	}
}

// An argument that is not a defined app is forwarded rather than judged: it may be a raw
// container name or a path the container runtime understands and the store does not.
func TestDelegate_UnknownAppFallsThroughToTheContainerRuntime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("PATH", "") // no runtime installed, so the forward fails in a recognisable way
	quiet(t)

	sto, err := store.Default()
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Dir(sto.Path("x")), 0o755)

	err = delegate(backend.New(sto), "stop", []string{"not-an-app"})
	if err == nil || !strings.Contains(err.Error(), "zcr") {
		t.Fatalf("an unknown name should be forwarded to the container runtime, got: %v", err)
	}
}

package firmware

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// An OS installed under UEFI records its boot entry in NVRAM, not only on disk: Windows
// Setup writes a "Windows Boot Manager" entry pointing at bootmgfw.efi. `zvr install`
// leaves those variables beside the disk it produced, and the app's first run must be
// seeded from them - copying the pristine OVMF template instead throws the boot entry away
// and leaves a freshly installed guest at the UEFI shell with no bootable device.
func TestInstalledVars_FoundBesideTheBaseImage(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "win11.qcow2")
	if err := os.WriteFile(base, []byte("disk"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := InstalledVars(base); got != "" {
		t.Errorf("with no variables beside the disk, want empty, got %q", got)
	}

	vars := base + ".uefi-vars.fd"
	if err := os.WriteFile(vars, []byte("nvram"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := InstalledVars(base); got != vars {
		t.Errorf("InstalledVars = %q, want the variables the install left at %q", got, vars)
	}
	if got := InstalledVars(""); got != "" {
		t.Errorf("an empty base image has no variables, got %q", got)
	}
}

// qemu's tpmdev emulator speaks swtpm's CONTROL protocol over the chardev it is given, and
// swtpm hands back the data channel through it. Pointed at swtpm's --server socket instead,
// qemu connects, sends a control request and blocks forever on a reply that channel will
// never send - at 0% CPU in unix_stream_data_wait, before it ever opens its window. The
// only symptom is a guest that appears to start and then does nothing at all.
func TestStartTPM_GivesQemuTheControlSocket(t *testing.T) {
	if _, err := exec.LookPath("swtpm"); err != nil {
		t.Skip("swtpm not installed")
	}
	dir := t.TempDir()
	// Kept short: a unix socket path has 108 bytes, and a temp dir plus a long name can
	// exceed it, which fails in a way that looks nothing like the real cause.
	runDir, err := os.MkdirTemp("/tmp", "ztpm")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(runDir)

	socketPath := filepath.Join(runDir, "t.sock")
	pidPath := filepath.Join(runDir, "t.pid")
	tpm, err := StartTPM(filepath.Join(dir, "state"), socketPath, pidPath)
	if err != nil {
		t.Fatalf("StartTPM: %v", err)
	}
	defer StopTPM(socketPath, pidPath)

	if tpm.SocketPath != socketPath {
		t.Errorf("SocketPath = %q, want the control socket at %q", tpm.SocketPath, socketPath)
	}
	// The socket qemu is handed must be the one swtpm is listening on for control, which
	// is exactly the path we passed - not a sibling with a suffix.
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("swtpm did not create the control socket qemu will be given: %v", err)
	}
	// And it must actually accept a connection, which is what qemu does first.
	conn, err := net.DialTimeout("unix", socketPath, 3*time.Second)
	if err != nil {
		t.Fatalf("the control socket does not accept connections: %v", err)
	}
	conn.Close()
}

package firmware

import (
	"encoding/binary"
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

// The legacy 2 MB OVMF build does not hand an attached TPM to the guest, and the failure is
// almost invisible: qemu publishes the TPM's ACPI device by itself, so the guest enumerates
// it and binds a driver to it, and only Windows notices there is nothing behind it - it reads
// TPMVersion 0 and reports that the PC does not meet Windows 11's requirements, naming no
// cause. Measured on Fedora 43: OVMF_CODE.secboot.fd gives TPMVersion 0 and a hard block,
// OVMF_CODE_4M.secboot.qcow2 gives 2 and no issue. So a host carrying both generations must
// never be handed the older one.
func TestOvmfSearch_PrefersBuildsThatHandOverTheTPM(t *testing.T) {
	for name, search := range map[string][]ovmfBuild{"UEFI": ovmfSearch, "Secure Boot": ovmfSecbootSearch} {
		seenLegacy := ""
		for _, build := range search {
			if !build.tpm {
				seenLegacy = build.code
				continue
			}
			if seenLegacy != "" {
				t.Errorf("%s search: %s hands over the TPM but is listed after %s, which does not",
					name, build.code, seenLegacy)
			}
		}
		if len(search) == 0 {
			t.Errorf("%s search list is empty", name)
		}
	}
}

// The two OVMF generations disagree on the size of the variable store - 2 MB code pairs with
// 128 KiB, 4 MB code with 512 KiB - and qemu handed a mismatched pair does not fail cleanly:
// it warns about the size and boots a guest whose Secure Boot state is quietly wrong. A store
// created before this host gained the 4 MB build is exactly that case.
func TestMatchesBuild_RejectsAStoreFromAnotherBuild(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy-vars.fd")
	current := filepath.Join(dir, "current-vars.fd")
	if err := os.WriteFile(legacy, make([]byte, 128*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, make([]byte, 512*1024), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := matchesBuild(legacy, current); err == nil {
		t.Error("a 128 KiB store against a 512 KiB template should be refused, not silently used")
	}
	if err := matchesBuild(current, current); err != nil {
		t.Errorf("a store matching its own template should be accepted, got %v", err)
	}
}

// qemu grows a qcow2 variable store as the guest writes to it, so the file size says nothing
// about what the firmware sees. Comparing file sizes would reject a perfectly good store as
// soon as the guest had booted once.
func TestPflashShape_ReportsTheQcow2VirtualSize(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "vars.qcow2")

	const virtualSize = 540672
	header := make([]byte, 512) // deliberately smaller than the virtual size it declares
	copy(header, "QFI\xfb")
	binary.BigEndian.PutUint64(header[24:32], virtualSize)
	if err := os.WriteFile(image, header, 0o600); err != nil {
		t.Fatal(err)
	}

	format, size, err := pflashShape(image)
	if err != nil {
		t.Fatal(err)
	}
	if format != "qcow2" {
		t.Errorf("format = %q, want qcow2", format)
	}
	if size != virtualSize {
		t.Errorf("size = %d, want the declared virtual size %d, not the file size %d", size, virtualSize, len(header))
	}

	raw := filepath.Join(dir, "vars.fd")
	if err := os.WriteFile(raw, make([]byte, 131072), 0o600); err != nil {
		t.Fatal(err)
	}
	format, size, err = pflashShape(raw)
	if err != nil {
		t.Fatal(err)
	}
	if format != "raw" || size != 131072 {
		t.Errorf("pflashShape(raw) = %q/%d, want raw/131072", format, size)
	}
}

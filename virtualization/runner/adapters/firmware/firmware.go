// Package firmware prepares the two host-side pieces a Windows-class guest needs before
// qemu starts: a per-app copy of the UEFI variable store, and a running TPM 2.0 emulator.
//
// Both are per-app on purpose. UEFI keeps boot entries in its variable store, and a TPM
// holds keys the guest believes are sealed to its own machine; sharing either between
// guests would let one app's state - or secrets - land in another.
package firmware

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

// OVMF images, in the order distributions put them. Secure Boot needs the matching pair:
// the code half carries the signature database, so a secboot VARS with plain CODE boots
// into an unusable state rather than a secured one.
var ovmfSearch = []struct{ code, vars string }{
	{"/usr/share/edk2/ovmf/OVMF_CODE.fd", "/usr/share/edk2/ovmf/OVMF_VARS.fd"},
	{"/usr/share/OVMF/OVMF_CODE.fd", "/usr/share/OVMF/OVMF_VARS.fd"},
	{"/usr/share/qemu/ovmf-x86_64-code.bin", "/usr/share/qemu/ovmf-x86_64-vars.bin"},
}

var ovmfSecbootSearch = []struct{ code, vars string }{
	{"/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd", "/usr/share/edk2/ovmf/OVMF_VARS.secboot.fd"},
	{"/usr/share/OVMF/OVMF_CODE.secboot.fd", "/usr/share/OVMF/OVMF_VARS.secboot.fd"},
}

// Prepare resolves the firmware for an app, copying the variable store on first use. The
// copy is what makes the guest's boot configuration persistent and its own.
//
// baseImage matters more than it looks. An OS installed under UEFI records its boot entry
// in NVRAM, not only on disk - Windows Setup writes a "Windows Boot Manager" entry pointing
// at bootmgfw.efi. `zvr install` leaves those variables beside the disk it produced, so
// when an app is first run against that disk its variable store is seeded from them.
// Copying the pristine OVMF template instead would throw the boot entry away and leave a
// freshly installed guest sitting at the UEFI shell with no bootable device.
func Prepare(virt schema.VirtualizationMeta, varsPath, baseImage string) (qemu.Firmware, error) {
	if virt.Firmware != schema.VMFirmwareUEFI {
		return qemu.Firmware{}, nil
	}

	search := ovmfSearch
	kind := "UEFI"
	if virt.SecureBoot {
		search, kind = ovmfSecbootSearch, "UEFI Secure Boot"
	}
	var code, template string
	for _, candidate := range search {
		if fileExists(candidate.code) && fileExists(candidate.vars) {
			code, template = candidate.code, candidate.vars
			break
		}
	}
	if code == "" {
		return qemu.Firmware{}, fmt.Errorf(
			"%s firmware (OVMF) not found on this host; install the edk2-ovmf package (Fedora) or ovmf (Debian), "+
				"or set Firmware: BIOS if the guest can boot that way", kind)
	}

	if !fileExists(varsPath) {
		if err := os.MkdirAll(filepath.Dir(varsPath), 0o755); err != nil {
			return qemu.Firmware{}, err
		}
		if installed := InstalledVars(baseImage); installed != "" && installed != varsPath {
			template = installed
		}
		data, err := os.ReadFile(template)
		if err != nil {
			return qemu.Firmware{}, fmt.Errorf("read the UEFI variable template: %w", err)
		}
		if err := os.WriteFile(varsPath, data, 0o600); err != nil {
			return qemu.Firmware{}, fmt.Errorf("create this app's UEFI variable store: %w", err)
		}
	}
	return qemu.Firmware{CodePath: code, VarsPath: varsPath}, nil
}

// InstalledVars is where `zvr install` leaves the UEFI variables belonging to a disk it
// installed: beside the disk, so they travel with it. Empty when there are none.
func InstalledVars(baseImage string) string {
	if baseImage == "" {
		return ""
	}
	candidate := baseImage + ".uefi-vars.fd"
	if fileExists(candidate) {
		return candidate
	}
	return ""
}

// TPM is a running swtpm process backing one guest's emulated TPM 2.0.
type TPM struct {
	SocketPath string
	pidPath    string
}

// StartTPM launches swtpm for an app and waits for its socket. Windows 11 refuses to
// install without a TPM, so a failure here has to be reported plainly rather than left for
// the installer to express as a vague compatibility complaint.
func StartTPM(stateDir, socketPath, pidPath string) (*TPM, error) {
	if _, err := exec.LookPath("swtpm"); err != nil {
		return nil, fmt.Errorf("swtpm not found on $PATH: an emulated TPM needs it (install the swtpm package), " +
			"or set TPM: false if the guest does not require one")
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	// A stale socket from a guest that died blocks the bind, the same way qemu's own
	// sockets do.
	_ = os.Remove(socketPath)

	command := exec.Command("swtpm", "socket",
		"--tpmstate", "dir="+stateDir,
		"--ctrl", "type=unixio,path="+socketPath+".ctrl",
		"--server", "type=unixio,path="+socketPath,
		"--tpm2",
		"--flags", "startup-clear",
		"--daemon",
		"--pid", "file="+pidPath,
	)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("start the TPM emulator: %w: %s", err, strings.TrimSpace(string(output)))
	}

	// --daemon returns immediately; the socket appears a moment later, and qemu fails hard
	// if it connects first.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fileExists(socketPath) {
			return &TPM{SocketPath: socketPath, pidPath: pidPath}, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil, fmt.Errorf("the TPM emulator did not create its socket at %s within 3s", socketPath)
}

// StopTPM terminates the emulator for an app and clears what it leaves behind. Called on
// every stop, including for guests that never had one, so a missing pidfile is not an
// error.
func StopTPM(socketPath, pidPath string) {
	if data, err := os.ReadFile(pidPath); err == nil {
		if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 && isSwtpm(pid) {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	_ = os.Remove(pidPath)
	_ = os.Remove(socketPath)
	_ = os.Remove(socketPath + ".ctrl")
}

// isSwtpm confirms a pid really is a TPM emulator before it is signalled. A stale pidfile
// outlives the process it names, pids are recycled, and the guest supervisor already
// refuses to signal on a pidfile alone - this is the same rule.
func isSwtpm(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ReplaceAll(string(data), "\x00", " "), "swtpm")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

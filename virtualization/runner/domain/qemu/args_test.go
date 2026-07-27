package qemu

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func testLayout() Layout {
	return Layout{
		Overlay: "/home/u/.local/share/zinc/vms/guest.qcow2",
		Seed:    "/home/u/.local/share/zinc/vms/guest-seed.iso",
		PIDFile: "/run/user/1000/zinc/vm/guest.pid",
		QMP:     "/run/user/1000/zinc/vm/guest.qmp",
		Serial:  "/run/user/1000/zinc/vm/guest.serial",
	}
}

func testCfg() schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincVirtualization,
		AppNameID:     "guest",
		ImageMeta:     schema.ImageMeta{Image: "/home/u/.local/share/zinc/images/fedora.qcow2"},
		VirtualizationMeta: schema.VirtualizationMeta{
			BaseDigest: "sha256:" + strings.Repeat("a", 64),
			MemoryMiB:  8192,
			VCPUs:      4,
			Display:    schema.VMDisplayAccelerated,
		},
	}
}

// pairs finds the value following each occurrence of flag.
func pairs(args []string, flag string) []string {
	var found []string
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag {
			found = append(found, args[index+1])
		}
	}
	return found
}

func has(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// The guest must actually be accelerated and sized as configured; without KVM these VMs
// are far too slow for what they are for.
func TestArgs_MachineSizingAndAcceleration(t *testing.T) {
	args := Args(testCfg(), testLayout())

	if args[0] != Binary {
		t.Errorf("argv[0] = %q, want %q", args[0], Binary)
	}
	if got := pairs(args, "-machine"); len(got) != 1 || !strings.Contains(got[0], "accel=kvm") {
		t.Errorf("-machine = %v, want it to enable kvm", got)
	}
	if got := pairs(args, "-cpu"); len(got) != 1 || got[0] != "host" {
		t.Errorf("-cpu = %v, want host", got)
	}
	if got := pairs(args, "-smp"); len(got) != 1 || got[0] != "4" {
		t.Errorf("-smp = %v, want 4", got)
	}
	if got := pairs(args, "-m"); len(got) != 1 || got[0] != "8192M" {
		t.Errorf("-m = %v, want 8192M", got)
	}
}

// -nodefaults keeps qemu from contributing devices of its own, so a guest's hardware is
// exactly what the config asked for. The seccomp sandbox is the host-side boundary.
func TestArgs_NoDefaultsAndSandbox(t *testing.T) {
	args := Args(testCfg(), testLayout())
	if !has(args, "-nodefaults") {
		t.Error("-nodefaults missing: qemu would add devices the config never asked for")
	}
	sandbox := pairs(args, "-sandbox")
	if len(sandbox) != 1 || !strings.HasPrefix(sandbox[0], "on") {
		t.Fatalf("-sandbox = %v, want it enabled", sandbox)
	}
	for _, deny := range []string{"elevateprivileges=deny", "spawn=deny", "resourcecontrol=deny"} {
		if !strings.Contains(sandbox[0], deny) {
			t.Errorf("-sandbox %q should contain %s", sandbox[0], deny)
		}
	}
}

// The guest writes to its overlay and only its overlay. The pinned base is reached
// through the overlay's backing file, so it must never be handed to qemu as a disk -
// which is what keeps the authored image from drifting away from its digest.
func TestArgs_OnlyTheOverlayIsAttached(t *testing.T) {
	cfg := testCfg()
	args := Args(cfg, testLayout())

	drives := pairs(args, "-drive")
	if len(drives) != 2 {
		t.Fatalf("got %d drives, want the overlay and the seed: %v", len(drives), drives)
	}
	if !strings.Contains(drives[0], testLayout().Overlay) || !strings.Contains(drives[0], "format=qcow2") {
		t.Errorf("first drive = %q, want the qcow2 overlay", drives[0])
	}
	for _, arg := range args {
		if strings.Contains(arg, cfg.ImageMeta.Image) {
			t.Errorf("the pinned base image appears in the command line (%q); only the overlay may be attached", arg)
		}
	}
}

// The seed is the guest's first-boot identity: read-only, because a guest has no business
// rewriting it, and absent entirely when cloud-init is off.
func TestArgs_SeedIsReadOnlyAndOptional(t *testing.T) {
	args := Args(testCfg(), testLayout())
	seed := pairs(args, "-drive")[1]
	if !strings.Contains(seed, "readonly=on") || !strings.Contains(seed, "format=raw") {
		t.Errorf("seed drive = %q, want a read-only raw disk", seed)
	}

	layout := testLayout()
	layout.Seed = ""
	if drives := pairs(Args(testCfg(), layout), "-drive"); len(drives) != 1 {
		t.Errorf("with cloud-init disabled, got %d drives, want only the overlay: %v", len(drives), drives)
	}
}

// A forwarded port must reach the host that started the guest, not the LAN. Binding every
// interface would quietly publish a guest service to the network.
func TestArgs_ForwardsBindLoopbackOnly(t *testing.T) {
	cfg := testCfg()
	cfg.VirtualizationMeta.ForwardPorts = []schema.PortForward{
		{HostPort: 2222, GuestPort: 22},
		{HostPort: 8080, GuestPort: 80},
	}
	netdev := pairs(Args(cfg, testLayout()), "-netdev")
	if len(netdev) != 1 {
		t.Fatalf("-netdev = %v, want exactly one", netdev)
	}
	for _, want := range []string{"hostfwd=tcp:127.0.0.1:2222-:22", "hostfwd=tcp:127.0.0.1:8080-:80"} {
		if !strings.Contains(netdev[0], want) {
			t.Errorf("-netdev %q should contain %q", netdev[0], want)
		}
	}
	if strings.Contains(netdev[0], "hostfwd=tcp::") {
		t.Error("a forward bound every interface; it must bind 127.0.0.1 so the guest port is not published to the LAN")
	}
}

// An app with no forwards still gets outbound access, and nothing inbound.
func TestArgs_NoForwardsMeansNoInbound(t *testing.T) {
	netdev := pairs(Args(testCfg(), testLayout()), "-netdev")
	if len(netdev) != 1 || netdev[0] != "user,id=net0" {
		t.Fatalf("-netdev = %v, want plain user networking with no forwards", netdev)
	}
}

// The display mode is the difference between a playable game and a slideshow, so each
// mode must produce exactly the device and display pair it promises.
func TestArgs_DisplayModes(t *testing.T) {
	cases := []struct {
		mode        schema.VMDisplay
		wantDevice  string
		wantDisplay string
	}{
		{schema.VMDisplayAccelerated, "virtio-gpu-gl-pci", "gtk,gl=on"},
		{schema.VMDisplayWindow, "virtio-gpu-pci", "gtk"},
		{schema.VMDisplayNone, "", "none"},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			cfg := testCfg()
			cfg.VirtualizationMeta.Display = tc.mode
			args := Args(cfg, testLayout())

			if got := pairs(args, "-display"); len(got) != 1 || got[0] != tc.wantDisplay {
				t.Errorf("-display = %v, want %q", got, tc.wantDisplay)
			}
			devices := pairs(args, "-device")
			if tc.wantDevice == "" {
				for _, device := range devices {
					if strings.Contains(device, "gpu") || strings.Contains(device, "tablet") {
						t.Errorf("headless mode should attach no display hardware, got %q", device)
					}
				}
				return
			}
			found := false
			for _, device := range devices {
				if strings.HasPrefix(device, tc.wantDevice) {
					found = true
				}
			}
			if !found {
				t.Errorf("devices = %v, want one starting with %q", devices, tc.wantDevice)
			}
			if !has(devices, "virtio-keyboard-pci") || !has(devices, "virtio-tablet-pci") {
				t.Errorf("a windowed guest needs a keyboard and pointer, got %v", devices)
			}
		})
	}
}

// Audio is an explicit grant, as it is for containers: an app that did not ask for sound
// gets no sound card at all rather than a silent one.
// venus is deliberately absent. Enabling it where the host virglrenderer lacks venus
// support does not degrade to OpenGL - it fails the whole renderer, leaving the guest with
// no display at all, which is exactly what happened when it was tried on Fedora 43.
func TestArgs_AcceleratedDoesNotRequireVenus(t *testing.T) {
	cfg := testCfg()
	cfg.VirtualizationMeta.Display = schema.VMDisplayAccelerated
	for _, device := range pairs(Args(cfg, testLayout()), "-device") {
		if strings.Contains(device, "venus") || strings.Contains(device, "blob=on") {
			t.Errorf("device %q enables venus/blob; on a host whose virglrenderer lacks venus this kills the display entirely", device)
		}
	}
}

func TestArgs_AudioOnlyOnGrant(t *testing.T) {
	cfg := testCfg()
	for _, arg := range Args(cfg, testLayout()) {
		if strings.Contains(arg, "audiodev") || strings.Contains(arg, "hda") {
			t.Fatalf("audio appeared without a grant: %q", arg)
		}
	}

	cfg.AudioMeta.Pipewire = true
	args := Args(cfg, testLayout())
	if got := pairs(args, "-audiodev"); len(got) != 1 || !strings.HasPrefix(got[0], "pipewire") {
		t.Errorf("-audiodev = %v, want pipewire", got)
	}
	if !has(pairs(args, "-device"), "hda-duplex,audiodev=snd0") {
		t.Error("granted audio should attach a sound device bound to the pipewire backend")
	}
}

// The control and console sockets are how zvr finds a running guest again; both must be
// non-blocking servers so a VM boots whether or not anyone is attached.
func TestArgs_ControlSocketsDoNotWaitForAClient(t *testing.T) {
	args := Args(testCfg(), testLayout())
	for _, flag := range []string{"-qmp", "-serial"} {
		got := pairs(args, flag)
		if len(got) != 1 {
			t.Fatalf("%s = %v, want exactly one", flag, got)
		}
		if !strings.Contains(got[0], "server=on") || !strings.Contains(got[0], "wait=off") {
			t.Errorf("%s %q must be a non-blocking server, or the guest waits for a client before booting", flag, got[0])
		}
	}
	if got := pairs(args, "-pidfile"); len(got) != 1 || got[0] != testLayout().PIDFile {
		t.Errorf("-pidfile = %v, want %q", got, testLayout().PIDFile)
	}
}

func TestDisplay_QuotesWhitespace(t *testing.T) {
	got := Display([]string{"qemu-system-x86_64", "-name", "my guest", "-m", "1024M"})
	if !strings.Contains(got, "'my guest'") {
		t.Errorf("Display() = %q, want the whitespace argument quoted", got)
	}
}

// Guest Vulkan is opt-in and costs the qemu process its seccomp jail: venus runs in a
// helper process that virglrenderer forks, and the sandbox both forbids the fork and kills
// the child that inherits its filter - silently, reported only as a generic "virgl could
// not be initialized". Measured on real hardware; this asserts the trade is made exactly
// where it was asked for and nowhere else.
func TestArgs_VulkanIsOptInAndDropsTheSandbox(t *testing.T) {
	cfg := testCfg()
	cfg.VirtualizationMeta.Display = schema.VMDisplayAccelerated

	// Default: sandboxed, and no venus.
	plain := Args(cfg, testLayout())
	if len(pairs(plain, "-sandbox")) != 1 {
		t.Error("an app that did not ask for Vulkan must keep qemu's seccomp sandbox")
	}
	for _, device := range pairs(plain, "-device") {
		if strings.Contains(device, "venus") {
			t.Errorf("venus must not appear without being asked for, got %q", device)
		}
	}

	// Opted in: venus present, sandbox gone.
	cfg.VirtualizationMeta.Vulkan = true
	vulkan := Args(cfg, testLayout())
	if len(pairs(vulkan, "-sandbox")) != 0 {
		t.Error("Vulkan requires the sandbox to be dropped, or the venus helper is killed")
	}
	var gpu string
	for _, device := range pairs(vulkan, "-device") {
		if strings.HasPrefix(device, "virtio-gpu-gl-pci") {
			gpu = device
		}
	}
	for _, want := range []string{"venus=on", "blob=on", "hostmem="} {
		if !strings.Contains(gpu, want) {
			t.Errorf("gpu device %q must contain %q", gpu, want)
		}
	}
}

// Vulkan rides on the accelerated display's device, so it must not silently attach itself
// to a mode that has no virtio-gpu-gl to carry it.
func TestArgs_VulkanOnlyOnTheAcceleratedDevice(t *testing.T) {
	cfg := testCfg()
	cfg.VirtualizationMeta.Vulkan = true
	for _, mode := range []schema.VMDisplay{schema.VMDisplayWindow, schema.VMDisplayNone} {
		cfg.VirtualizationMeta.Display = mode
		for _, device := range pairs(Args(cfg, testLayout()), "-device") {
			if strings.Contains(device, "venus") {
				t.Errorf("display %q should carry no venus, got %q", mode, device)
			}
		}
	}
}

// A Windows-class guest is a different machine, not a Linux one with a flag flipped: it
// boots UEFI, has a TPM, and can only use hardware whose drivers ship in the box. Windows
// Setup pointed at a virtio disk reports finding no drives at all.
func TestArgs_WindowsClassMachine(t *testing.T) {
	cfg := testCfg()
	cfg.VirtualizationMeta.Display = schema.VMDisplayCompatible
	cfg.VirtualizationMeta.Devices = schema.VMDevicesCompatible
	cfg.VirtualizationMeta.Firmware = schema.VMFirmwareUEFI
	cfg.VirtualizationMeta.TPM = true
	layout := testLayout()
	layout.Firmware = Firmware{CodePath: "/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd", VarsPath: "/data/guest-uefi-vars.fd"}
	layout.TPMSocket = "/run/user/1000/zinc/vm/guest.tpm"

	args := Args(cfg, layout)

	// UEFI: two pflash drives, and the code half read-only so the guest cannot rewrite the
	// firmware it booted from.
	flash := pairs(args, "-drive")
	var code, vars string
	for _, drive := range flash {
		if strings.Contains(drive, "if=pflash") && strings.Contains(drive, "unit=0") {
			code = drive
		}
		if strings.Contains(drive, "if=pflash") && strings.Contains(drive, "unit=1") {
			vars = drive
		}
	}
	if code == "" || !strings.Contains(code, "readonly=on") {
		t.Errorf("UEFI code pflash should be attached read-only, got %q", code)
	}
	if vars == "" || !strings.Contains(vars, layout.Firmware.VarsPath) {
		t.Errorf("the guest needs its own writable UEFI variable store, got %q", vars)
	}

	// TPM 2.0 on the interface Windows looks for.
	if !has(pairs(args, "-device"), "tpm-tis,tpmdev=tpm0") {
		t.Error("a TPM guest needs the tpm-tis device Windows expects")
	}
	if got := pairs(args, "-tpmdev"); len(got) != 1 || !strings.Contains(got[0], "emulator") {
		t.Errorf("-tpmdev = %v, want the swtpm emulator backend", got)
	}

	// Devices an installer without virtio drivers can actually see.
	devices := pairs(args, "-device")
	for _, want := range []string{"ahci,id=ahci", "e1000e,netdev=net0", "usb-tablet,bus=usb.0"} {
		if !has(devices, want) {
			t.Errorf("a compatible-devices guest needs %q, got %v", want, devices)
		}
	}
	for _, device := range devices {
		if strings.HasPrefix(device, "virtio-") {
			t.Errorf("a compatible-devices guest must carry no virtio hardware, got %q", device)
		}
	}
	// Drives too, not only -device. `if=virtio` creates a virtio-blk controller without
	// ever appearing in a -device argument, which is exactly how a virtio cloud-init seed
	// slipped onto a machine configured for a guest with no virtio drivers.
	for _, drive := range pairs(args, "-drive") {
		if strings.Contains(drive, "if=virtio") {
			t.Errorf("a compatible-devices guest must have no virtio drive, got %q", drive)
		}
	}
	if got := pairs(args, "-display"); len(got) != 1 || got[0] != "gtk" {
		t.Errorf("-display = %v, want plain gtk on VGA", got)
	}

	// Windows reads the hardware clock as local time; UTC leaves it wrong by the offset.
	if got := pairs(args, "-rtc"); len(got) != 1 || got[0] != "base=localtime" {
		t.Errorf("-rtc = %v, want base=localtime for a compatible-devices guest", got)
	}
}

// A Linux guest must be untouched by any of that: still virtio, still BIOS, still UTC.
func TestArgs_LinuxGuestUnchangedByWindowsSupport(t *testing.T) {
	args := Args(testCfg(), testLayout())
	if len(pairs(args, "-tpmdev")) != 0 {
		t.Error("a guest that did not ask for a TPM must not get one")
	}
	for _, drive := range pairs(args, "-drive") {
		if strings.Contains(drive, "pflash") {
			t.Errorf("a BIOS guest should have no pflash, got %q", drive)
		}
	}
	if got := pairs(args, "-rtc"); len(got) != 1 || got[0] != "base=utc" {
		t.Errorf("-rtc = %v, want base=utc for a virtio guest", got)
	}
	if !has(pairs(args, "-device"), "virtio-net-pci,netdev=net0") {
		t.Error("a virtio guest should keep its virtio NIC")
	}
}

// Install media is attached only for an install, and read-only: the installer boots from
// the disc, and a normal run of the same app must not see it at all.
func TestArgs_InstallMediaOnlyWhenInstalling(t *testing.T) {
	cfg := testCfg()
	cfg.VirtualizationMeta.Devices = schema.VMDevicesCompatible
	cfg.VirtualizationMeta.InstallMedia = []string{"/iso/win11.iso", "/iso/virtio-win.iso"}

	normal := Args(cfg, testLayout())
	for _, drive := range pairs(normal, "-drive") {
		if strings.Contains(drive, "cdrom") {
			t.Errorf("a normal run must not attach install media, got %q", drive)
		}
	}
	if len(pairs(normal, "-boot")) != 0 {
		t.Error("a normal run should not override the boot order")
	}

	layout := testLayout()
	layout.Installing = true
	installing := Args(cfg, layout)
	cdroms := 0
	for _, drive := range pairs(installing, "-drive") {
		if strings.Contains(drive, "media=cdrom") {
			cdroms++
			if !strings.Contains(drive, "readonly=on") {
				t.Errorf("install media should be read-only, got %q", drive)
			}
		}
	}
	if cdroms != 2 {
		t.Errorf("got %d cdroms, want both install ISOs", cdroms)
	}
	// once=, not order=: an installer reboots itself partway through, and a permanently
	// disc-first order lands that reboot back in the installer.
	got := pairs(installing, "-boot")
	if len(got) != 1 || !strings.Contains(got[0], "once=d") {
		t.Errorf("-boot = %v, want a one-shot disc boot", got)
	}
	if strings.Contains(got[0], "order=d") {
		t.Errorf("-boot = %v, a permanent disc-first order restarts the installer on its own reboot", got)
	}
}

// The cloud-init seed follows the machine's device profile. A virtio seed on a compatible
// guest is doubly wrong: unreadable to a guest with no virtio drivers, and virtio hardware
// on a machine configured precisely because it has none.
func TestArgs_SeedFollowsTheDeviceProfile(t *testing.T) {
	cfg := testCfg()
	cfg.VirtualizationMeta.Devices = schema.VMDevicesCompatible
	cfg.VirtualizationMeta.Display = schema.VMDisplayCompatible
	args := Args(cfg, testLayout()) // testLayout carries a seed

	seedAttached := false
	for _, drive := range pairs(args, "-drive") {
		if strings.Contains(drive, testLayout().Seed) {
			seedAttached = true
			if strings.Contains(drive, "if=virtio") {
				t.Errorf("the seed should not be a virtio drive on a compatible guest: %q", drive)
			}
			if !strings.Contains(drive, "readonly=on") {
				t.Errorf("the seed must stay read-only: %q", drive)
			}
		}
	}
	if !seedAttached {
		t.Error("the seed should still be attached, just on a bus the guest can read")
	}
	if !has(pairs(args, "-device"), "ide-cd,drive=seed,bus=ahci.1") {
		t.Error("the compatible seed should ride the same controller as the disk")
	}

	// A virtio guest keeps the virtio seed.
	cfg.VirtualizationMeta.Devices = schema.VMDevicesVirtio
	cfg.VirtualizationMeta.Display = schema.VMDisplayAccelerated
	virtioSeed := false
	for _, drive := range pairs(Args(cfg, testLayout()), "-drive") {
		if strings.Contains(drive, testLayout().Seed) && strings.Contains(drive, "if=virtio") {
			virtioSeed = true
		}
	}
	if !virtioSeed {
		t.Error("a virtio guest should keep its virtio seed drive")
	}
}

// Secure Boot is not enabled by loading the secboot firmware. OVMF keeps its signature
// database in memory only System Management Mode may write, so without SMM the variables
// are unprotected and the firmware does not ENFORCE Secure Boot. Nothing on the host shows
// the difference; the guest decides it. Windows 11 Setup refuses with "This PC doesn't
// currently meet Windows 11 system requirements", because as far as it can tell Secure Boot
// is simply off.
func TestArgs_SecureBootRequiresSMM(t *testing.T) {
	cfg := testCfg()
	cfg.VirtualizationMeta.Firmware = schema.VMFirmwareUEFI
	cfg.VirtualizationMeta.SecureBoot = true
	args := Args(cfg, testLayout())

	machine := pairs(args, "-machine")
	if len(machine) != 1 || !strings.Contains(machine[0], "smm=on") {
		t.Errorf("-machine = %v, want smm=on or Secure Boot is not enforced", machine)
	}
	globals := pairs(args, "-global")
	if !has(globals, "driver=cfi.pflash01,property=secure,value=on") {
		t.Errorf("-global = %v, want the pflash secure property that routes variable writes through SMM", globals)
	}
	if !has(globals, "ICH9-LPC.disable_s3=1") {
		t.Errorf("-global = %v, want S3 disabled: resuming from it bypasses the firmware's re-validation", globals)
	}

	// A guest that did not ask for Secure Boot pays none of it.
	cfg.VirtualizationMeta.SecureBoot = false
	plain := Args(cfg, testLayout())
	if got := pairs(plain, "-machine"); strings.Contains(got[0], "smm") {
		t.Errorf("-machine = %v, want no SMM without Secure Boot", got)
	}
	if len(pairs(plain, "-global")) != 0 {
		t.Error("a guest without Secure Boot should carry no secure-boot globals")
	}
}

// Fedora ships its current 4 MB OVMF build as a pair of qcow2 images and keeps only the
// legacy 2 MB build as raw .fd files. Hardcoding format=raw makes qemu read a qcow2 header
// as if it were firmware, so the format has to travel with the paths.
func TestFirmwareArgs_CarriesTheImageFormat(t *testing.T) {
	qcow := firmwareArgs(Firmware{
		CodePath: "/usr/share/edk2/ovmf/OVMF_CODE_4M.secboot.qcow2",
		VarsPath: "/data/guest-uefi-vars.qcow2",
		Format:   "qcow2",
	})
	for _, drive := range pairs(qcow, "-drive") {
		if !strings.Contains(drive, "format=qcow2") {
			t.Errorf("pflash drive %q should declare format=qcow2", drive)
		}
	}

	// An unset format still has to produce a working machine: raw is what every .fd is.
	raw := firmwareArgs(Firmware{
		CodePath: "/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd",
		VarsPath: "/data/guest-uefi-vars.fd",
	})
	for _, drive := range pairs(raw, "-drive") {
		if !strings.Contains(drive, "format=raw") {
			t.Errorf("with no format set, pflash drive %q should default to raw", drive)
		}
	}
}

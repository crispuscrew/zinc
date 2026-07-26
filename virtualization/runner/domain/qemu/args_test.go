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

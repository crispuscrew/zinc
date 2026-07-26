// Package qemu builds the command line for a VM app. It is pure - a validated config and
// a set of resolved paths in, argv out, no I/O - which is what lets zvr print the exact
// command with --dry-run before anything boots, the same promise zcr makes for podman.
//
// Everything the guest gets is stated here explicitly. The machine is started with
// -nodefaults so qemu contributes no devices of its own: a VM's hardware is exactly what
// the config asked for, never a default that happens to be compiled in.
package qemu

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Binary is the emulator zvr drives. Only x86_64 guests are supported: a foreign
// architecture would run without KVM and be far too slow for the interactive use these
// VMs are for.
const Binary = "qemu-system-x86_64"

// Layout is where one app's files live, resolved by the caller so this stays pure.
type Layout struct {
	Overlay string // the app's copy-on-write disk, backed by the pinned base image
	Seed    string // cloud-init seed ISO; empty when cloud-init is disabled
	PIDFile string // qemu writes its pid here, which is how zvr finds it again
	QMP     string // control socket: status queries and a graceful power button
	Serial  string // the guest's serial console, for `zvr console`
}

// Args returns the full argv for cfg. The caller has already validated cfg, so the sizing
// and display mode are known good.
func Args(cfg schema.AppConfig, layout Layout) []string {
	virt := cfg.VirtualizationMeta
	args := []string{
		Binary,
		"-name", cfg.AppNameID,
		// q35 is the modern chipset (PCIe, no legacy baggage); KVM is what makes the guest
		// fast enough to interact with, and -cpu host exposes the real CPU's features so
		// guest code is not held back by a generic model.
		"-machine", "q35,accel=kvm",
		"-cpu", "host",
		"-smp", strconv.Itoa(virt.VCPUs),
		"-m", strconv.FormatInt(virt.MemoryMiB, 10) + "M",
		"-nodefaults",
		"-rtc", "base=utc",
		// qemu's own seccomp jail. The host process is the boundary between a guest and
		// this machine, so it gives up what it does not need: spawning helpers, raising
		// privileges, changing its own scheduling.
		"-sandbox", "on,obsolete=deny,elevateprivileges=deny,spawn=deny,resourcecontrol=deny",
		"-pidfile", layout.PIDFile,
		"-qmp", "unix:" + layout.QMP + ",server=on,wait=off",
		// server=on,wait=off: the socket exists from the start but the guest never waits
		// for anyone to attach, so a VM boots whether or not you are watching its console.
		"-serial", "unix:" + layout.Serial + ",server=on,wait=off",
	}

	args = append(args, diskArgs(layout)...)
	args = append(args, netArgs(virt.ForwardPorts)...)
	args = append(args, displayArgs(virt.Display)...)
	args = append(args, audioArgs(cfg.AudioMeta)...)
	return args
}

// diskArgs attaches the app's overlay and, when present, the cloud-init seed. The overlay
// is the only writable disk: the pinned base it is backed by is never opened for writing,
// so the authored image cannot drift from its digest.
func diskArgs(layout Layout) []string {
	args := []string{
		"-drive", "file=" + layout.Overlay + ",if=virtio,format=qcow2,discard=unmap",
	}
	if layout.Seed != "" {
		// Read-only and raw: cloud-init looks for a filesystem labelled cidata, and the
		// guest has no business writing to its own seed.
		args = append(args, "-drive", "file="+layout.Seed+",if=virtio,format=raw,readonly=on")
	}
	return args
}

// netArgs gives the guest user-mode networking: outbound access through qemu's own NAT,
// with no host interface to attach to and nothing inbound except the forwards asked for.
// Each forward binds 127.0.0.1 rather than every interface, so a forwarded guest port
// reaches the host that started it and not the LAN.
func netArgs(forwards []schema.PortForward) []string {
	netdev := "user,id=net0"
	for _, forward := range forwards {
		netdev += fmt.Sprintf(",hostfwd=tcp:127.0.0.1:%d-:%d", forward.HostPort, forward.GuestPort)
	}
	return []string{
		"-netdev", netdev,
		"-device", "virtio-net-pci,netdev=net0",
	}
}

// displayArgs wires up how the VM is seen. Accelerated is the point of the whole design:
// virtio-gpu-gl renders guest 3D on the host GPU and hands the result to the compositor
// as a dmabuf, so frames never leave the machine and never get encoded - the difference
// between a playable game and a slideshow.
func displayArgs(mode schema.VMDisplay) []string {
	switch mode {
	case schema.VMDisplayAccelerated:
		// OpenGL only, through virgl. Guest VULKAN would need qemu's venus=on, which is
		// deliberately NOT set here: it requires a host virglrenderer built with venus
		// support, and where that is missing the renderer fails to initialize at all -
		// taking working OpenGL down with it and leaving a guest with no display, rather
		// than degrading to what it could still have done. Measured on Fedora 43, whose
		// virglrenderer 1.2.0 ships with no venus support: "failed to initialize venus
		// renderer / virgl could not be initialized".
		//
		// The consequence is worth stating plainly: a guest's Vulkan runs on llvmpipe,
		// which is the CPU. See the README's known limits.
		return append(inputArgs(), "-device", "virtio-gpu-gl-pci", "-display", "gtk,gl=on")
	case schema.VMDisplayWindow:
		return append(inputArgs(), "-device", "virtio-gpu-pci", "-display", "gtk")
	default: // VMDisplayNone
		// No window and no graphics card at all; the serial console is the way in.
		return []string{"-display", "none"}
	}
}

// inputArgs gives a windowed guest its keyboard and pointer. The tablet reports absolute
// coordinates, so the pointer tracks the host's without the window having to grab it -
// grabbing is still there (qemu binds it) for anything that wants relative motion.
func inputArgs() []string {
	return []string{
		"-device", "virtio-keyboard-pci",
		"-device", "virtio-tablet-pci",
	}
}

// audioArgs routes guest audio to pipewire when the app asked for it. An app that did not
// gets no sound card at all, rather than a silent one: the grant is explicit, as it is for
// containers.
func audioArgs(audio schema.AudioMeta) []string {
	if !audio.Pipewire {
		return nil
	}
	return []string{
		"-audiodev", "pipewire,id=snd0",
		// intel-hda is the device every guest OS already has a driver for, which matters
		// more here than the marginal efficiency of a virtio sound device.
		"-device", "intel-hda",
		"-device", "hda-duplex,audiodev=snd0",
	}
}

// Display renders the argv for a human, lightly quoting anything with whitespace. It is
// what --dry-run prints, so the operator sees the real command rather than a summary.
func Display(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			quoted[index] = "'" + arg + "'"
			continue
		}
		quoted[index] = arg
	}
	return strings.Join(quoted, " ")
}

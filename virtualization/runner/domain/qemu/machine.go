package qemu

import (
	"strconv"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// The parts of a guest's machine that differ by what the guest OS can actually drive. A
// Linux cloud image boots on BIOS and speaks virtio to everything; Windows 11 refuses to
// install without UEFI and a TPM, and its installer has drivers for neither a virtio disk
// nor a virtio NIC - pointed at one it reports finding no drives at all. So these are
// separate, explicit fields rather than a "Windows" preset: the config says what the
// machine has, and the guest either drives it or does not.

// machineType builds the -machine argument. Secure Boot needs SMM: the firmware keeps its
// signature database in memory that only System Management Mode may write, and without SMM
// the variables are not protected, so OVMF runs but does not ENFORCE Secure Boot. The
// distinction is invisible from the host and decisive in the guest - Windows 11 reports
// that the PC does not meet its requirements, because as far as it can tell Secure Boot is
// switched off.
func machineType(virt schema.VirtualizationMeta) string {
	machine := "q35,accel=kvm"
	if virt.SecureBoot {
		machine += ",smm=on"
	}
	return machine
}

// secureBootArgs completes what machineType starts. The pflash "secure" property is what
// actually routes writes to the variable store through SMM, and S3 suspend is disabled
// because resuming from it bypasses the firmware's own re-validation.
func secureBootArgs(virt schema.VirtualizationMeta) []string {
	if !virt.SecureBoot {
		return nil
	}
	return []string{
		"-global", "driver=cfi.pflash01,property=secure,value=on",
		"-global", "ICH9-LPC.disable_s3=1",
	}
}

// Firmware is where the OVMF images live on the host, and the per-app copy of the writable
// variable store. UEFI keeps boot entries in that store, so each app needs its own: a
// shared one would let one guest's boot configuration overwrite another's.
type Firmware struct {
	CodePath string // read-only OVMF code
	VarsPath string // this app's writable variable store
	Format   string // pflash image format, "raw" or "qcow2"; empty means raw
}

// firmwareArgs attaches OVMF as a pair of pflash drives. The code half is read-only, so a
// guest cannot rewrite the firmware it booted from.
//
// The format is not always raw. Fedora ships its current 4 MB build as a pair of qcow2
// images and keeps only the legacy 2 MB build as raw .fd files, so the format travels with
// the paths rather than being assumed.
func firmwareArgs(firmware Firmware) []string {
	if firmware.CodePath == "" {
		return nil // BIOS: qemu's built-in SeaBIOS needs nothing said about it
	}
	format := firmware.Format
	if format == "" {
		format = "raw"
	}
	return []string{
		"-drive", "if=pflash,format=" + format + ",unit=0,readonly=on,file=" + firmware.CodePath,
		"-drive", "if=pflash,format=" + format + ",unit=1,file=" + firmware.VarsPath,
	}
}

// tpmArgs attaches an emulated TPM 2.0 over the socket a swtpm process is listening on.
// tpm-tis is the interface Windows expects to find.
func tpmArgs(socketPath string) []string {
	if socketPath == "" {
		return nil
	}
	return []string{
		"-chardev", "socket,id=chrtpm,path=" + socketPath,
		"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
		"-device", "tpm-tis,tpmdev=tpm0",
	}
}

// diskArgsFor attaches the app's writable disk with an interface the guest can drive.
// virtio is faster in every dimension; AHCI is what an installer that has never heard of
// virtio can actually see.
func diskArgsFor(devices schema.VMDevices, overlay string) []string {
	if devices == schema.VMDevicesCompatible {
		return []string{"-drive", "file=" + overlay + ",if=none,id=disk0,format=qcow2,discard=unmap",
			"-device", "ahci,id=ahci",
			"-device", "ide-hd,drive=disk0,bus=ahci.0"}
	}
	return []string{"-drive", "file=" + overlay + ",if=virtio,format=qcow2,discard=unmap"}
}

// netDeviceFor picks the NIC. e1000e is an Intel gigabit part that every mainstream OS has
// had a driver for since long before it was installed.
func netDeviceFor(devices schema.VMDevices) string {
	if devices == schema.VMDevicesCompatible {
		return "e1000e,netdev=net0"
	}
	return "virtio-net-pci,netdev=net0"
}

// inputArgsFor gives a windowed guest a keyboard and pointer it can drive. A USB tablet
// needs no driver anywhere and reports absolute coordinates, so the pointer tracks the
// host's without the window having to grab it.
func inputArgsFor(devices schema.VMDevices) []string {
	if devices == schema.VMDevicesCompatible {
		return []string{"-device", "qemu-xhci,id=usb", "-device", "usb-tablet,bus=usb.0", "-device", "usb-kbd,bus=usb.0"}
	}
	return []string{"-device", "virtio-keyboard-pci", "-device", "virtio-tablet-pci"}
}

// mediaArgs attaches install ISOs as read-only CD-ROMs, in the order given, so a Windows
// install can carry both its installer and a driver disc.
func mediaArgs(media []string) []string {
	var args []string
	for index, path := range media {
		id := "cd" + strconv.Itoa(index)
		args = append(args,
			"-drive", "file="+path+",if=none,id="+id+",media=cdrom,readonly=on",
			"-device", "ide-cd,drive="+id+",bus=ide."+strconv.Itoa(index))
	}
	return args
}

// rtcArgs sets the guest's clock. Windows reads the hardware clock as local time and will
// otherwise sit at the wrong time by the size of the timezone offset.
func rtcArgs(devices schema.VMDevices) []string {
	if devices == schema.VMDevicesCompatible {
		return []string{"-rtc", "base=localtime"}
	}
	return []string{"-rtc", "base=utc"}
}

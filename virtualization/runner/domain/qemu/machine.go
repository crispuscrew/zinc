package qemu

import (
	"crypto/sha256"
	"fmt"
	"strconv"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// The parts of a guest's machine that differ by what the guest OS can actually drive. A
// Linux cloud image boots on BIOS and speaks virtio to everything; Windows 11 refuses to
// install without UEFI and a TPM, and its installer has drivers for neither a virtio disk
// nor a virtio NIC - pointed at one it reports finding no drives at all. So these are
// separate, explicit fields rather than a "Windows" preset: the config says what the
// machine has, and the guest either drives it or does not.

// identityArgs gives the guest a machine identity of its own. Without -uuid every qemu
// guest reports the SMBIOS UUID 00000000-0000-0000-0000-000000000000, and the default NIC
// carries the MAC 52:54:00:12:34:56 - values shared with every other default qemu VM in the
// world. That is not a cosmetic detail: Windows Autopilot identifies a device by a hash
// built from exactly these fields, so a freshly installed guest can match a stranger's
// corporate enrolment and come up at OOBE demanding a sign-in to their tenant, branded with
// their logo. Observed here: a Windows 11 install landed on an SAP sign-in page.
//
// The identity is derived from the app name rather than randomised, so it survives a reset
// and a reinstall. Windows treats a machine whose UUID changed as different hardware and
// wants reactivating, which a randomised value would trigger on every boot.
func identityArgs(appName string) []string {
	sum := sha256.Sum256([]byte("zinc/vm/" + appName))

	// RFC 4122 layout: version 4 in the high nibble of byte 6, variant 10 in byte 8. The
	// bytes are a hash rather than random, but the shape has to be a well-formed UUID for
	// firmware and guests to accept it.
	var uuid [16]byte
	copy(uuid[:], sum[:16])
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return []string{"-uuid", fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])}
}

// macFor picks this app's NIC address. An override is used verbatim - validation has already
// screened it - so an app that must not look like a QEMU guest can present something else.
// Otherwise the address is derived under 52:54:00, QEMU's own assigned OUI, which keeps it
// recognisably a virtual machine's while making the host part per-app; the same shape libvirt
// uses.
func macFor(appName, override string) string {
	if override != "" {
		return override
	}
	sum := sha256.Sum256([]byte("zinc/vm/mac/" + appName))
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", sum[0], sum[1], sum[2])
}

// compatibleDisplayDevice picks the graphics for a guest with no display driver of its own.
// Such a guest keeps whatever mode the firmware left it in, so the resolution is decided here
// and never changes: resizing the window only scales those pixels.
//
// Plain VGA cannot be told a resolution - it has no xres/yres, and its built-in EDID is
// 1280x800, which is why an unconfigured guest is always exactly that size. bochs-display
// takes one and the firmware honours it. virtio-vga and qxl-vga accept the same properties
// but are no use here, because OVMF drives their VGA-compatible half and settles back to
// 1280x800.
//
// The framebuffer size and EDID refresh rate come with the mode, and both are load-bearing:
// left at the device's defaults a 4K guest has less video memory than its screen needs AND an
// EDID pixel clock that overflows its own field, and either one alone drops it silently back
// to 1280x800.
//
// The cost is that bochs-display has no VGA compatibility at all, so a BIOS guest given it
// gets no picture. Validation requires UEFI alongside a fixed size; this keeps VGA for
// everyone who did not ask for one.
func compatibleDisplayDevice(virt schema.VirtualizationMeta) string {
	if virt.Firmware != schema.VMFirmwareUEFI {
		return "VGA,vgamem_mb=64"
	}
	mode, ok := schema.GuestDisplay(virt.DisplayWidth, virt.DisplayHeight)
	if !ok {
		// Unset, or a size validation would have refused. Either way plain VGA is what a
		// guest that never asked for one has always had.
		return "VGA,vgamem_mb=64"
	}
	return fmt.Sprintf("bochs-display,xres=%d,yres=%d,vgamem=%d,refresh_rate=%d",
		virt.DisplayWidth, virt.DisplayHeight, mode.VideoMemBytes, mode.RefreshMilliHz)
}

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

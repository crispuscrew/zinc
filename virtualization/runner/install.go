package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/firmware"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

// `zvr install` produces a base disk by running an OS installer, for guests that have no
// cloud image to start from - Windows above all.
//
// It deliberately takes flags rather than an app name. An app config pins its base image by
// digest, and a disk that does not exist yet has no digest, so requiring one here would be
// a chicken-and-egg: you could not author the app until the disk existed, and could not
// create the disk without the app. Install first, pin the result, then author the app
// against it. That also keeps the rule that a pinned base is never written to intact -
// installation is how the base comes into being, not something done to a base that already
// has one.
const installUsage = `usage: zvr install --disk PATH --media ISO [--media ISO]... [options]

  --disk PATH        the disk to install onto; created if missing
  --size GiB         size of a newly created disk (default 64)
  --media ISO        installer image, repeatable - Windows also needs the virtio-win ISO
                     if you intend to switch it to virtio devices afterwards
  --memory MiB       guest RAM during the install (default 4096)
  --vcpus N          guest CPUs during the install (default 4)
  --firmware F       BIOS or UEFI (default UEFI; Windows 11 requires UEFI)
  --secure-boot      enable UEFI Secure Boot (Windows 11 expects it)
  --tpm              attach an emulated TPM 2.0 (Windows 11 requires one)
  --devices D        Virtio or Compatible (default Compatible, which is what an
                     installer without virtio drivers can actually see)`

type mediaList []string

func (list *mediaList) String() string { return strings.Join(*list, ",") }
func (list *mediaList) Set(value string) error {
	*list = append(*list, value)
	return nil
}

func cmdInstall(argv []string) error {
	fset := flag.NewFlagSet("install", flag.ContinueOnError)
	disk := fset.String("disk", "", "the disk to install onto; created if missing")
	size := fset.Int64("size", 64, "size in GiB of a newly created disk")
	memory := fset.Int64("memory", 4096, "guest RAM in MiB during the install")
	vcpus := fset.Int("vcpus", 4, "guest CPUs during the install")
	firmwareKind := fset.String("firmware", string(schema.VMFirmwareUEFI), "BIOS or UEFI")
	secureBoot := fset.Bool("secure-boot", false, "enable UEFI Secure Boot")
	tpm := fset.Bool("tpm", false, "attach an emulated TPM 2.0")
	devices := fset.String("devices", string(schema.VMDevicesCompatible), "Virtio or Compatible")
	var media mediaList
	fset.Var(&media, "media", "installer ISO (repeatable)")
	if err := fset.Parse(argv); err != nil {
		return err
	}
	if *disk == "" || len(media) == 0 {
		return fmt.Errorf("%s", installUsage)
	}
	diskPath, err := filepath.Abs(*disk)
	if err != nil {
		return err
	}
	for index, path := range media {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if _, err := os.Stat(absolute); err != nil {
			return fmt.Errorf("install media %s: %w", absolute, err)
		}
		media[index] = absolute
	}

	if _, err := os.Stat(diskPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
			return err
		}
		create := exec.Command("qemu-img", "create", "-f", "qcow2", diskPath, strconv.FormatInt(*size, 10)+"G")
		if output, err := create.CombinedOutput(); err != nil {
			return fmt.Errorf("create %s: %w: %s", diskPath, err, strings.TrimSpace(string(output)))
		}
		fmt.Printf("created %s (%d GiB, grows as it is used)\n", diskPath, *size)
	} else {
		// Refuse to silently install over something: this writes to the disk directly.
		fmt.Printf("installing onto the existing disk %s\n", diskPath)
	}

	cfg := schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincVirtualization,
		AppNameID:     "install",
		VirtualizationMeta: schema.VirtualizationMeta{
			MemoryMiB:    *memory,
			VCPUs:        *vcpus,
			Display:      schema.VMDisplayCompatible,
			Firmware:     schema.VMFirmware(*firmwareKind),
			SecureBoot:   *secureBoot,
			TPM:          *tpm,
			Devices:      schema.VMDevices(*devices),
			InstallMedia: media,
		},
	}

	layout, cleanup, err := installLayout(cfg, diskPath)
	if err != nil {
		return err
	}
	defer cleanup()

	args := qemu.Args(cfg, layout)
	fmt.Println("starting the installer; the guest's window is qemu's own.")
	fmt.Println("when the install finishes, shut the guest down from inside and this returns.")

	// Foreground, unlike `run`: an install is one interactive sitting, and the command
	// ending when the guest powers off is exactly the signal the next step needs.
	command := exec.Command(args[0], args[1:]...)
	command.Stdout, command.Stderr, command.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := command.Run(); err != nil {
		return fmt.Errorf("the installer exited with an error: %w", err)
	}

	digest, err := installedDigest(diskPath)
	if err != nil {
		return err
	}
	fmt.Printf("\ninstall finished. %s is now a base image.\n\n", diskPath)
	fmt.Println("Author an app against it:")
	fmt.Printf("  zc new <name> --vm --image %s \\\n", diskPath)
	fmt.Printf("      --base-digest %s \\\n", digest)
	fmt.Printf("      --memory %d --vcpus %d --devices %s --display Compatible", *memory, *vcpus, *devices)
	if *firmwareKind == string(schema.VMFirmwareUEFI) {
		fmt.Print(" --firmware UEFI")
	}
	if *secureBoot {
		fmt.Print(" --secure-boot")
	}
	if *tpm {
		fmt.Print(" --tpm")
	}
	fmt.Println()
	fmt.Println("\nEvery run then writes to its own overlay, so `zvr reset` returns the app")
	fmt.Println("to the state it is in right now.")
	return nil
}

// installLayout resolves the host-side pieces for an install: the disk itself stands in for
// the overlay (there is no base to layer over yet), with firmware and TPM state kept beside
// it so a half-finished install can be resumed.
func installLayout(cfg schema.AppConfig, diskPath string) (qemu.Layout, func(), error) {
	layout := paths.Paths{}
	resolved, err := paths.Default()
	if err != nil {
		return qemu.Layout{}, nil, err
	}
	layout = resolved
	if err := layout.EnsureDirs(); err != nil {
		return qemu.Layout{}, nil, err
	}

	// Named after the disk so two installs at once do not collide, and so resuming one
	// finds its own firmware state.
	name := "install-" + strings.TrimSuffix(filepath.Base(diskPath), filepath.Ext(diskPath))
	machine := qemu.Layout{
		Overlay:    diskPath,
		PIDFile:    layout.PIDFile(name),
		QMP:        layout.QMP(name),
		Serial:     layout.Serial(name),
		Installing: true,
	}

	prepared, err := firmware.Prepare(cfg.VirtualizationMeta, diskPath+".uefi-vars.fd")
	if err != nil {
		return qemu.Layout{}, nil, err
	}
	machine.Firmware = prepared

	cleanup := func() {}
	if cfg.VirtualizationMeta.TPM {
		firmware.StopTPM(layout.TPMSocket(name), layout.TPMPID(name))
		if _, err := firmware.StartTPM(diskPath+".tpm-state", layout.TPMSocket(name), layout.TPMPID(name)); err != nil {
			return qemu.Layout{}, nil, err
		}
		machine.TPMSocket = layout.TPMSocket(name)
		cleanup = func() { firmware.StopTPM(layout.TPMSocket(name), layout.TPMPID(name)) }
	}
	return machine, cleanup, nil
}

// installedDigest hashes the finished disk so the operator can paste the pin straight into
// an app config.
func installedDigest(diskPath string) (string, error) {
	digest, err := diskDigest(diskPath)
	if err != nil {
		return "", fmt.Errorf("hash the installed disk: %w", err)
	}
	return digest, nil
}

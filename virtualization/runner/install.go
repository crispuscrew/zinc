package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/disk"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

const installUsage = `usage: zvr install --disk PATH --media ISO [--media ISO]...
  --size GiB          new disk size (default 64)
  --memory MiB        RAM (default 4096)
  --vcpus N          CPU count (default 4)
  --firmware F       UEFI (default) or BIOS
  --secure-boot      require UEFI Secure Boot
  --tpm              emulated TPM 2.0
  --devices PROFILE  Compatible (default) or Virtio
  --resolution WxH   compatible display size (UEFI only)
  --resume           explicitly authorize writing an existing installer disk
  --app APP          read shared settings and CreatorFlags from an app
  --creator-arg ARG  raw installer-QEMU argv element (repeatable)
  --dry-run          print QEMU argv; create no files or helpers`

func cmdInstall(argv []string) error {
	fset := flag.NewFlagSet("install", flag.ContinueOnError)
	destination := fset.String("disk", "", "installation target")
	size := fset.Int64("size", 64, "new disk GiB")
	memory := fset.Int64("memory", 4096, "RAM MiB")
	cores := fset.Int("vcpus", 4, "CPU count")
	firmwareKind := fset.String("firmware", "UEFI", "UEFI or BIOS")
	secure := fset.Bool("secure-boot", false, "Secure Boot")
	tpm := fset.Bool("tpm", false, "TPM 2.0")
	devices := fset.String("devices", "Compatible", "device profile")
	resolution := fset.String("resolution", "", "WxH")
	resume := fset.Bool("resume", false, "authorize existing target")
	dry := fset.Bool("dry-run", false, "plan only")
	appName := fset.String("app", "", "app providing shared intent and CreatorFlags")
	var media, flags mediaList
	fset.Var(&media, "media", "ISO; repeatable")
	fset.Var(&flags, "creator-arg", "raw installer QEMU argument; repeatable")
	if err := fset.Parse(argv); err != nil {
		return err
	}
	if fset.NArg() != 0 || *destination == "" || len(media) == 0 {
		return fmt.Errorf("%s", installUsage)
	}
	if *firmwareKind != "UEFI" && *firmwareKind != "BIOS" {
		return fmt.Errorf("--firmware: require UEFI or BIOS")
	}
	width, height, err := parseResolution(*resolution)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(*destination)
	if err != nil {
		return err
	}
	identity := sha256.Sum256([]byte(path))
	cfg := schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincVirtualization,
		AppNameID:       fmt.Sprintf("install-%x", identity[:8]),
		ResourcesMeta:   schema.ResourcesMeta{MaxRamMiB: *memory, MaxCPUCores: float64(*cores)},
		StartConditions: schema.StartConditions{LoaderBIOS: *firmwareKind == "BIOS", SecureBoot: *secure, TPM: *tpm},
		DisplayMeta:     schema.DisplayMeta{DisableGpuAccess: true, DisplayWidth: width, DisplayHeight: height}}
	if *appName != "" {
		if err := installApp(&cfg, *appName, fset); err != nil {
			return err
		}
	}
	cfg.CreatorFlags = append(cfg.CreatorFlags, flags...)
	runtime := vmoptions.Default(cfg.AppNameID, path)
	runtime.Display, runtime.Devices = vmoptions.DisplayCompatible, vmoptions.Devices(*devices)
	runtime.DiskSizeGiB, runtime.InstallMedia = *size, media
	for index, medium := range runtime.InstallMedia {
		absolute, err := filepath.Abs(medium)
		if err != nil {
			return err
		}
		runtime.InstallMedia[index] = absolute
	}
	if err := validateInstall(cfg, runtime); err != nil {
		return err
	}
	layout, err := planInstall(cfg, runtime)
	if err != nil {
		return err
	}
	for _, warning := range qemu.Warnings(cfg, runtime, true) {
		fmt.Fprintln(os.Stderr, "warning: "+warning)
	}
	if *dry {
		if len(cfg.CreatorFlags) > 0 {
			fmt.Printf("# %d raw CreatorFlags omitted from plan\n", len(cfg.CreatorFlags))
		}
		fmt.Println(qemu.Display(qemu.PlanArgs(cfg, layout)))
		return nil
	}
	cleanup, err := prepareInstall(cfg, runtime, layout, *resume)
	if err != nil {
		return err
	}
	defer cleanup()
	args := qemu.Args(cfg, layout)
	command := exec.Command(args[0], args[1:]...)
	command.Stdout, command.Stderr, command.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := command.Run(); err != nil {
		return fmt.Errorf("installer exited: %w", err)
	}
	digest, err := disk.Digest(path)
	if err != nil {
		return err
	}
	fmt.Printf("installed base: %s\nBaseDigest for separate runtime options: %s\n", path, strings.TrimSpace(digest))
	return nil
}

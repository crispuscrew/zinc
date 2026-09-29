// Package firmware resolves and prepares per-instance firmware and TPM state.
package firmware

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

type Plan struct {
	Firmware qemu.Firmware
	Template string
}

// Resolve performs only reads, including when the variable store does not exist.
func Resolve(start schema.StartConditions, varsPath, baseImage string) (Plan, error) {
	if start.LoaderBIOS {
		if start.SecureBoot {
			return Plan{}, fmt.Errorf("SecureBoot requires UEFI")
		}
		return Plan{}, nil
	}
	search := ovmfSearch
	if start.SecureBoot {
		search = ovmfSecbootSearch
	}
	var build ovmfBuild
	for _, candidate := range search {
		if fileExists(candidate.code) && fileExists(candidate.vars) {
			build = candidate
			break
		}
	}
	if build.code == "" {
		return Plan{}, fmt.Errorf("matching UEFI firmware (OVMF) not found; install edk2-ovmf/ovmf or explicitly select LoaderBIOS")
	}
	if start.TPM && !build.tpm {
		return Plan{}, fmt.Errorf("TPM requires a current OVMF build with TCG2 support; found only %s", build.code)
	}
	plan := Plan{Firmware: qemu.Firmware{CodePath: build.code, VarsPath: varsPath, Format: build.format}}
	if _, err := os.Lstat(varsPath); err == nil {
		return plan, matchesBuild(varsPath, build.vars)
	} else if !os.IsNotExist(err) {
		return Plan{}, err
	}
	plan.Template = build.vars
	if installed := InstalledVars(baseImage); installed != "" && installed != varsPath {
		if start.SecureBoot {
			return Plan{}, fmt.Errorf("SecureBoot: installed variable store %s is not pinned or trusted; explicitly provision trusted firmware state", installed)
		}
		if err := matchesBuild(installed, build.vars); err != nil {
			return Plan{}, err
		}
		plan.Template = installed
	}
	return plan, nil
}

func Prepare(start schema.StartConditions, varsPath, baseImage string) (qemu.Firmware, error) {
	plan, err := Resolve(start, varsPath, baseImage)
	if err != nil || plan.Template == "" {
		return plan.Firmware, err
	}
	if err := os.MkdirAll(filepath.Dir(varsPath), 0o700); err != nil {
		return qemu.Firmware{}, err
	}
	data, err := os.ReadFile(plan.Template)
	if err != nil {
		return qemu.Firmware{}, err
	}
	file, err := os.CreateTemp(filepath.Dir(varsPath), ".nvram-*")
	if err != nil {
		return qemu.Firmware{}, err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return qemu.Firmware{}, err
	}
	if err := os.Link(file.Name(), varsPath); err != nil {
		return qemu.Firmware{}, fmt.Errorf("publish firmware state without replacing existing data: %w", err)
	}
	return plan.Firmware, nil
}

func InstalledVars(baseImage string) string {
	if baseImage != "" && fileExists(baseImage+".uefi-vars.fd") {
		return baseImage + ".uefi-vars.fd"
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Package disk prepares pinned bases, persistent overlays and provisioning discs.
package disk

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type imageInfo struct {
	Format      string `json:"format"`
	VirtualSize int64  `json:"virtual-size"`
	Backing     string `json:"full-backing-filename"`
	BackingName string `json:"backing-filename"`
	BackingType string `json:"backing-filename-format"`
}

func inspect(path string) (imageInfo, error) {
	var info imageInfo
	output, err := exec.Command("qemu-img", "info", "--output=json", "-f", "qcow2", path).CombinedOutput()
	if err != nil {
		return info, fmt.Errorf("inspect disk %s: %w: %s", path, err, strings.TrimSpace(string(output)))
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return info, fmt.Errorf("inspect disk %s: %w", path, err)
	}
	if info.Format != "qcow2" || info.VirtualSize <= 0 {
		return info, fmt.Errorf("disk %s: require a valid qcow2 image", path)
	}
	return info, nil
}

// EnsureOverlay never replaces or resizes an existing guest disk.
func EnsureOverlay(base, digest, overlay string, sizeGiB int64) error {
	if err := VerifyBase(base, digest); err != nil {
		return err
	}
	baseInfo, err := inspect(base)
	if err != nil {
		return err
	}
	if sizeGiB < 0 || sizeGiB > (1<<63-1)/(1<<30) {
		return fmt.Errorf("invalid disk size")
	}
	requested := sizeGiB * (1 << 30)
	if requested != 0 && requested < baseInfo.VirtualSize {
		return fmt.Errorf("requested disk size is smaller than the base; shrinking is never automatic")
	}
	if info, err := os.Lstat(overlay); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("overlay %s is not a regular file", overlay)
		}
		return verifyOverlay(base, overlay, requested, baseInfo.VirtualSize)
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(overlay), ".overlay-*.qcow2")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Close(); err != nil {
		return err
	}
	args := []string{"create", "-f", "qcow2", "-F", "qcow2", "-b", base, file.Name()}
	if sizeGiB > 0 {
		args = append(args, strconv.FormatInt(sizeGiB, 10)+"G")
	}
	if output, err := exec.Command("qemu-img", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("create overlay: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Chmod(file.Name(), 0o600); err != nil {
		return err
	}
	if err := os.Link(file.Name(), overlay); err != nil {
		return fmt.Errorf("publish overlay without replacing existing data: %w", err)
	}
	return nil
}

func verifyOverlay(base, overlay string, requested, baseSize int64) error {
	info, err := inspect(overlay)
	if err != nil {
		return err
	}
	backing := info.Backing
	if backing == "" {
		backing = info.BackingName
		if backing != "" && !filepath.IsAbs(backing) {
			backing = filepath.Join(filepath.Dir(overlay), backing)
		}
	}
	basePath, baseErr := filepath.EvalSymlinks(base)
	backingPath, backingErr := filepath.EvalSymlinks(backing)
	if err := errors.Join(baseErr, backingErr); err != nil {
		return fmt.Errorf("resolve overlay backing file: %w", err)
	}
	if basePath != backingPath || info.BackingType != "qcow2" {
		return fmt.Errorf("existing overlay has a different backing image or format; refusing automatic rebase")
	}
	if info.VirtualSize < baseSize || requested != 0 && requested != info.VirtualSize {
		return fmt.Errorf("existing overlay size differs; use an explicit offline resize instead of changing runtime options")
	}
	return nil
}

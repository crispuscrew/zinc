package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

const maximumManifestBytes = 1 << 20

func ManifestPath(app string) (string, error) {
	if !domain.ValidID(app) {
		return "", fmt.Errorf("invalid app identity %q", app)
	}
	root := os.Getenv("ZINC_NETWORK_MANIFEST_DIR")
	if root == "" {
		runtime := os.Getenv("XDG_RUNTIME_DIR")
		if runtime == "" {
			return "", fmt.Errorf("provisioned network requires XDG_RUNTIME_DIR or ZINC_NETWORK_MANIFEST_DIR")
		}
		root = filepath.Join(runtime, "zinc", "network")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("manifest directory must be absolute")
	}
	return filepath.Join(root, app+".json"), nil
}

func Load(cfg schema.AppConfig) (Manifest, error) {
	path, err := ManifestPath(cfg.AppNameID)
	if err != nil {
		return Manifest{}, err
	}
	manifest, err := LoadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("%s requires preprovisioned packet-preserving topology: %w", cfg.AppNameID, err)
	}
	if manifest.AppNameID != cfg.AppNameID {
		return manifest, fmt.Errorf("manifest app identity mismatch")
	}
	return manifest, nil
}

func LoadFile(path string) (Manifest, error) {
	var manifest Manifest
	if err := secureParents(filepath.Dir(path)); err != nil {
		return manifest, err
	}
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return manifest, fmt.Errorf("open manifest %s: %w", path, err)
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return manifest, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || !owned(info) {
		return manifest, fmt.Errorf("manifest must be owner/root-owned regular file, not group/world writable")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumManifestBytes+1))
	if err != nil {
		return manifest, err
	}
	if len(data) > maximumManifestBytes {
		return manifest, fmt.Errorf("manifest exceeds size limit")
	}
	manifest, err = Decode(data)
	if err != nil {
		return manifest, err
	}
	if err := namespace(manifest.NetworkNamespace, manifest.NetworkInode, "net"); err != nil {
		return manifest, err
	}
	if err := namespace(manifest.UserNamespace, manifest.UserInode, "user"); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func Decode(data []byte) (Manifest, error) {
	var manifest Manifest
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return manifest, fmt.Errorf("network manifest must be a JSON object")
	}
	if err := uniqueKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return manifest, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode network manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return manifest, fmt.Errorf("manifest must contain exactly one JSON document")
	}
	return manifest, nil
}

func owned(info os.FileInfo) bool {
	stat, valid := info.Sys().(*syscall.Stat_t)
	return valid && (stat.Uid == uint32(os.Getuid()) || stat.Uid == 0)
}

func secureParents(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || !owned(info) || info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("unsafe manifest parent %s", path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

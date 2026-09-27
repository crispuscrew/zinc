package network

import (
	"fmt"
	"path/filepath"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func ValidateBinding(cfg schema.AppConfig, manifest Manifest) error {
	if manifest.Version != ManifestVersion || manifest.AppNameID != cfg.AppNameID || !domain.ValidID(manifest.Generation) {
		return fmt.Errorf("manifest version, app identity or generation mismatch")
	}
	if !manifest.PacketPreserving || !manifest.Exclusive || !manifest.StaticNeighbors || !manifest.CompleteInventory {
		return fmt.Errorf("require exclusive packet-preserving topology, static neighbors and complete host/app inventory")
	}
	if normalizePolicy(manifest.Policy, manifest.Topology.Interfaces) != normalizePolicy(cfg.NetworkMeta, manifest.Topology.Interfaces) {
		return fmt.Errorf("provisioned policy differs from app config; reprovision before launch")
	}
	for _, path := range []string{manifest.NetworkNamespace, manifest.UserNamespace} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("namespace paths must be absolute and clean")
		}
	}
	if manifest.NetworkInode == 0 || manifest.UserInode == 0 {
		return fmt.Errorf("namespace inode identities are required")
	}
	want := domain.Container
	if cfg.Type == schema.ZincVirtualization {
		want = domain.VirtualMachine
	}
	if manifest.Topology.Mode != want {
		return fmt.Errorf("%s requires %s topology", cfg.Type, want)
	}
	if cfg.InternalUserMeta.KeepUserID && !manifest.KeepUserID {
		return fmt.Errorf("provisioned user namespace does not declare keep-id mapping")
	}
	if err := validatePublications(manifest); err != nil {
		return err
	}
	return domain.ValidateTopology(domain.Resolved{Config: cfg, Topology: manifest.Topology})
}

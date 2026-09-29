package netns

import (
	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

// Configure binds provisioned MACs in a runtime-only copy and returns the exact
// TAP list. The caller must not save this resolved copy over the authored app.
func Configure(cfg schema.AppConfig) (schema.AppConfig, []qemu.NetworkAttachment, error) {
	if !Applies(cfg) {
		return cfg, nil, nil
	}
	manifest, err := provision.Load(cfg)
	if err != nil {
		return cfg, nil, err
	}
	return ConfigureResolved(cfg, manifest)
}

func ConfigureResolved(cfg schema.AppConfig, manifest provision.Manifest) (schema.AppConfig, []qemu.NetworkAttachment, error) {
	// Validate topology without resolving domains; CommandResolved validates the
	// complete policy after the caller supplies its approved resolver.
	if err := provision.ValidateBinding(cfg, manifest); err != nil {
		return cfg, nil, err
	}
	bound := cfg
	bound.NetworkMeta.Interfaces = append([]schema.NetworkInterface{}, cfg.NetworkMeta.Interfaces...)
	var attachments []qemu.NetworkAttachment
	for index, iface := range bound.NetworkMeta.Interfaces {
		for _, entry := range manifest.Topology.Interfaces {
			if iface.ID == entry.InterfaceID {
				if iface.MacAddress == "" {
					bound.NetworkMeta.Interfaces[index].MacAddress = entry.MAC
				}
				attachments = append(attachments, qemu.NetworkAttachment{InterfaceID: iface.ID, TapName: entry.Device})
			}
		}
	}
	return bound, attachments, nil
}

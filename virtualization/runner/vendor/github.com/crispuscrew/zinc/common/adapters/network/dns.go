package network

import (
	"fmt"

	"github.com/crispuscrew/zinc/common/adapters/dnsproxy"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

// ReadyDNS binds the manifest's DNS promise to an authenticated running proxy.
// It opens only its private status socket; it never starts a service or listener.
func ReadyDNS(cfg schema.AppConfig, manifest Manifest) error {
	if len(cfg.NetworkMeta.DNS.ResolversByPriority) == 0 {
		if manifest.DNSControlSocket != "" {
			return fmt.Errorf("DNS control socket requires configured resolvers")
		}
		return nil
	}
	if manifest.DNSControlSocket == "" {
		return fmt.Errorf("configured DNS requires dns_control_socket for authenticated proxy readiness")
	}
	if err := dnsproxy.CheckReady(manifest.DNSControlSocket, cfg.NetworkMeta.DNS, manifest.DNSProxyAddresses); err != nil {
		return fmt.Errorf("DNS proxy readiness: %w", err)
	}
	return nil
}

// Package network loads owner-provisioned topology. It never creates host links,
// routes, namespaces, listeners, or privileged services.
package network

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

const ManifestVersion = 1

type Manifest struct {
	Version           int                `json:"version"`
	AppNameID         string             `json:"app_name_id"`
	Generation        string             `json:"generation"`
	NetworkNamespace  string             `json:"network_namespace"`
	UserNamespace     string             `json:"user_namespace"`
	NetworkInode      uint64             `json:"network_inode"`
	UserInode         uint64             `json:"user_inode"`
	PacketPreserving  bool               `json:"packet_preserving"`
	Exclusive         bool               `json:"exclusive"`
	StaticNeighbors   bool               `json:"static_neighbors"`
	CompleteInventory bool               `json:"complete_inventory"`
	KeepUserID        bool               `json:"keep_user_id"`
	Policy            schema.NetworkMeta `json:"policy"`
	Topology          domain.Topology    `json:"topology"`
	DNSProxyAddresses []string           `json:"dns_proxy_addresses"`
	DNSConfigDigest   string             `json:"dns_config_digest"`
	DNSControlSocket  string             `json:"dns_control_socket"`
	Publications      []Publication      `json:"publications"`
}

// Lookup must use the supplied configured resolvers. No implementation defaults
// to net.DefaultResolver. Results are a launch snapshot, frozen with the policy.
type Lookup func(schema.DNSMeta, string) ([]netip.Addr, error)

type Plan struct {
	Manifest  Manifest
	Resolved  domain.Resolved
	Upstreams []domain.Upstream
}

func DNSDigest(meta schema.DNSMeta) string {
	encoded, _ := json.Marshal(meta) // this schema contains no unsupported JSON types
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func Resolve(cfg schema.AppConfig, manifest Manifest, lookup Lookup) (Plan, error) {
	plan := Plan{Manifest: manifest, Resolved: domain.Resolved{Config: cfg, Topology: manifest.Topology, Domains: domain.DomainSets{}, Generation: manifest.Generation}}
	if err := ValidateBinding(cfg, manifest); err != nil {
		return plan, err
	}
	upstreams, err := domain.Upstreams(cfg.NetworkMeta.DNS)
	if err != nil {
		return plan, err
	}
	plan.Upstreams = upstreams
	if len(upstreams) == 0 && len(manifest.DNSProxyAddresses) > 0 {
		return plan, fmt.Errorf("DNS proxy requires explicitly configured resolvers")
	}
	if len(upstreams) > 0 {
		if len(manifest.DNSProxyAddresses) == 0 || manifest.DNSConfigDigest != DNSDigest(cfg.NetworkMeta.DNS) {
			return plan, fmt.Errorf("configured DNS requires a provisioned proxy matching DNSConfigDigest; direct/encrypted DNS cannot be written as plaintext resolv.conf")
		}
	}
	for _, value := range manifest.DNSProxyAddresses {
		address, err := netip.ParseAddr(value)
		if err != nil || address.Is4In6() || address.Zone() != "" || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() {
			return plan, fmt.Errorf("invalid DNS proxy address %q", value)
		}
	}
	if err := resolveDomains(&plan.Resolved, lookup); err != nil {
		return plan, err
	}
	return plan, domain.Validate(plan.Resolved)
}

func normalizePolicy(meta schema.NetworkMeta, attachments []domain.Attachment) string {
	meta.Interfaces = append([]schema.NetworkInterface{}, meta.Interfaces...)
	for index, iface := range meta.Interfaces {
		for _, attachment := range attachments {
			if iface.ID == attachment.InterfaceID && iface.MacAddress == "" {
				meta.Interfaces[index].MacAddress = attachment.MAC
			}
		}
	}
	encoded, _ := json.Marshal(meta)
	return string(encoded)
}

package validate

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

var domainRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

func validDomain(domain string) bool {
	if len(domain) > 253 || !domainRE.MatchString(domain) {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) > 63 {
			return false
		}
	}
	return true
}

func checkDNS(dns schema.DNSMeta, add addFunc) {
	for index, resolver := range dns.ResolversByPriority {
		field := fmt.Sprintf("NetworkMeta.DNS.ResolversByPriority[%d]", index)
		switch resolver.Protocol {
		case schema.DNSUDP, schema.DNSTCP, schema.DNSTLS, schema.DNSHTTPS, schema.DNSQUIC:
		default:
			add("%s.Protocol %q: must be UDP, TCP, TLS, HTTPS or QUIC", field, resolver.Protocol)
		}
		host, valid := dnsEndpoint(resolver.Endpoint)
		if !valid {
			add("%s.Endpoint %q: must be an IP or plain hostname, optionally with a numeric port 1-65535 (IPv6 ports require brackets); no scheme or path", field, resolver.Endpoint)
		} else if !literalIP(host) && len(resolver.BootstrapIPs) == 0 {
			add("%s.BootstrapIPs: named endpoints require literal bootstrap IPs; implicit host DNS is not permitted", field)
		}
		for position, address := range resolver.BootstrapIPs {
			if !literalIP(address) {
				add("%s.BootstrapIPs[%d] %q: must be a literal IP address without a zone, port or CIDR", field, position, address)
			}
		}
		if resolver.Path != "" {
			if resolver.Protocol != schema.DNSHTTPS {
				add("%s.Path: only supported with HTTPS", field)
			}
			parsed, err := url.ParseRequestURI(resolver.Path)
			if err != nil || hasUnsafe(resolver.Path) || !strings.HasPrefix(resolver.Path, "/") ||
				strings.HasPrefix(resolver.Path, "//") || strings.ContainsAny(resolver.Path, "?#\\") ||
				(parsed != nil && (parsed.IsAbs() || hasControl(parsed.Path))) {
				add("%s.Path %q: must be an absolute HTTP path without a scheme, authority, query, fragment or control characters", field, resolver.Path)
			}
		}
	}
}

func literalIP(value string) bool {
	address, err := netip.ParseAddr(value)
	return err == nil && address.Zone() == "" && !address.Is4In6()
}

// A missing port selects the protocol's standard port, never a host resolver.
func dnsEndpoint(endpoint string) (string, bool) {
	if literalIP(endpoint) {
		return endpoint, true
	}
	host := endpoint
	if strings.ContainsAny(endpoint, ":[]") {
		var port string
		var err error
		host, port, err = net.SplitHostPort(endpoint)
		if err != nil || port == "" || strings.Trim(port, "0123456789") != "" {
			return "", false
		}
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", false
		}
	}
	if literalIP(host) {
		return host, true
	}
	// A malformed dotted IPv4 literal must not become a hostname with bootstrap.
	if strings.Trim(host, "0123456789.") == "" {
		return "", false
	}
	return host, validDomain(host)
}

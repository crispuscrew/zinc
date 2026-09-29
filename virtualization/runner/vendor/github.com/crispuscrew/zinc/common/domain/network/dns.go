package network

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Upstream describes transport precisely. It is NOT an application firewall grant.
type Upstream struct {
	Protocol  schema.DNSProtocol
	Transport schema.NetworkProtocol
	Host      string
	Addresses []string
	Port      int
	Path      string
}

func Upstreams(meta schema.DNSMeta) ([]Upstream, error) {
	var result []Upstream
	for index, resolver := range meta.ResolversByPriority {
		upstream, err := upstream(resolver)
		if err != nil {
			return nil, fmt.Errorf("resolver[%d]: %w", index, err)
		}
		result = append(result, upstream)
	}
	return result, nil
}

func upstream(resolver schema.DNSResolver) (Upstream, error) {
	result := Upstream{Protocol: resolver.Protocol, Host: resolver.Endpoint, Path: resolver.Path, Port: 53, Transport: schema.NetworkTCP}
	switch resolver.Protocol {
	case schema.DNSUDP:
		result.Transport = schema.NetworkUDP
	case schema.DNSTCP:
	case schema.DNSTLS:
		result.Port = 853
	case schema.DNSHTTPS:
		result.Port = 443
		if result.Path == "" {
			result.Path = "/dns-query"
		}
	case schema.DNSQUIC:
		result.Port, result.Transport = 853, schema.NetworkUDP
	default:
		return result, fmt.Errorf("unknown DNS protocol %q", resolver.Protocol)
	}
	if _, err := netip.ParseAddr(resolver.Endpoint); err != nil && strings.ContainsAny(resolver.Endpoint, ":[]") {
		host, port, err := net.SplitHostPort(resolver.Endpoint)
		if err != nil {
			return result, fmt.Errorf("invalid endpoint %q", resolver.Endpoint)
		}
		result.Host = host
		result.Port, err = strconv.Atoi(port)
		if err != nil || result.Port < 1 || result.Port > 65535 {
			return result, fmt.Errorf("invalid resolver port")
		}
	}
	if result.Path != "" {
		parsed, err := url.ParseRequestURI(result.Path)
		if resolver.Protocol != schema.DNSHTTPS || err != nil || parsed.IsAbs() || !strings.HasPrefix(result.Path, "/") || strings.HasPrefix(result.Path, "//") || strings.ContainsAny(result.Path, "?#\\\r\n\t ") {
			return result, fmt.Errorf("Path requires an absolute HTTPS path")
		}
	}
	if address, err := netip.ParseAddr(result.Host); err == nil {
		if address.Zone() != "" || address.Is4In6() {
			return result, fmt.Errorf("invalid resolver address")
		}
		result.Addresses = []string{address.String()}
	} else {
		if !ValidDomain(result.Host) || len(resolver.BootstrapIPs) == 0 {
			return result, fmt.Errorf("hostname requires explicit bootstrap IPs; host DNS fallback is forbidden")
		}
		result.Addresses = append([]string{}, resolver.BootstrapIPs...)
	}
	for _, value := range resolver.BootstrapIPs {
		address, err := netip.ParseAddr(value)
		if err != nil || address.Zone() != "" || address.Is4In6() {
			return result, fmt.Errorf("invalid bootstrap address %q", value)
		}
	}
	return result, nil
}

func ValidDomain(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return strings.Trim(value, "0123456789.") != ""
}

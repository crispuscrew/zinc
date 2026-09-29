// Package dnsproxy resolves only through explicitly configured DNS upstreams.
package dnsproxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

const (
	queryTimeout   = 10 * time.Second
	attemptTimeout = 2 * time.Second
	lookupTimeout  = 30 * time.Second
	maxUpstreams   = 16
	maxAddresses   = 16
	maxAliases     = 8
)

// Resolver is immutable after New and safe for concurrent use. TLS uses system CAs.
type Resolver struct {
	upstreams []domain.Upstream
	roots     *x509.CertPool
	attempt   time.Duration
}

func New(meta schema.DNSMeta) (*Resolver, error) {
	upstreams, err := domain.Upstreams(meta)
	if err != nil {
		return nil, err
	}
	if len(upstreams) == 0 || len(upstreams) > maxUpstreams {
		return nil, fmt.Errorf("DNS requires 1..%d configured resolvers", maxUpstreams)
	}
	for _, upstream := range upstreams {
		if upstream.Protocol == schema.DNSQUIC && upstream.Port == 53 {
			return nil, fmt.Errorf("DNS QUIC must not use port 53 (RFC 9250)")
		}
		if len(upstream.Addresses) == 0 || len(upstream.Addresses) > maxAddresses {
			return nil, fmt.Errorf("DNS requires 1..%d upstream addresses", maxAddresses)
		}
	}
	return &Resolver{upstreams: upstreams, attempt: attemptTimeout}, nil
}

// Exchange tries configured resolvers and their bootstrap addresses in order.
// Only NOERROR and NXDOMAIN are terminal; truncation never triggers implicit TCP.
func (resolver *Resolver) Exchange(ctx context.Context, query *dns.Msg) (*dns.Msg, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var failures []error
	for _, upstream := range resolver.upstreams {
		for _, address := range upstream.Addresses {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			request := query.Copy()
			request.Id = dns.Id()
			if upstream.Protocol == schema.DNSQUIC {
				request.Id = 0
			}
			attempt, stop := context.WithTimeout(ctx, resolver.attempt)
			endpoint := net.JoinHostPort(address, strconv.Itoa(upstream.Port))
			answer, err := resolver.exchange(attempt, upstream, endpoint, request)
			stop()
			if err == nil {
				err = validateAnswer(request, answer)
			}
			if err == nil {
				answer.Id = query.Id
				if upstream.Protocol == schema.DNSQUIC {
					restoreOptions(query, answer)
				}
				return answer, nil
			}
			failures = append(failures, fmt.Errorf("%s %s: %w", upstream.Protocol, endpoint, err))
		}
	}
	return nil, fmt.Errorf("all configured DNS upstreams failed: %w", errors.Join(failures...))
}

func (resolver *Resolver) exchange(ctx context.Context, upstream domain.Upstream, endpoint string, query *dns.Msg) (*dns.Msg, error) {
	switch upstream.Protocol {
	case schema.DNSHTTPS:
		return resolver.https(ctx, upstream, endpoint, query)
	case schema.DNSQUIC:
		return resolver.quic(ctx, upstream, endpoint, query)
	default:
		return resolver.classic(ctx, upstream, endpoint, query)
	}
}

func (resolver *Resolver) tlsConfig(host string) *tls.Config {
	return &tls.Config{ServerName: host, RootCAs: resolver.roots, MinVersion: tls.VersionTLS12}
}

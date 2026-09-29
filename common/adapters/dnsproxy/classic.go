package dnsproxy

import (
	"context"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

func (resolver *Resolver) classic(ctx context.Context, upstream domain.Upstream, endpoint string, query *dns.Msg) (*dns.Msg, error) {
	client := &dns.Client{Net: "tcp", UDPSize: dns.MaxMsgSize, Timeout: resolver.attempt}
	if upstream.Protocol == schema.DNSUDP {
		client.Net = "udp"
	}
	if upstream.Protocol == schema.DNSTLS {
		client.Net, client.TLSConfig = "tcp-tls", resolver.tlsConfig(upstream.Host)
	}
	connection, err := client.DialContext(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	stop := context.AfterFunc(ctx, func() { connection.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if err := connection.WriteMsg(query); err != nil {
		return nil, err
	}
	return connection.ReadMsg()
}

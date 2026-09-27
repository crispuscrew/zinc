package dnsproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/netip"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/miekg/dns"
	quic "github.com/quic-go/quic-go"
)

const (
	doqProtocolError    quic.ApplicationErrorCode = 2
	doqRequestCancelled quic.StreamErrorCode      = 3
	maxQUICWindow                                 = dns.MaxMsgSize + 3 // Two framing bytes and one excess-byte probe.
)

func (resolver *Resolver) quic(ctx context.Context, upstream domain.Upstream, endpoint string, query *dns.Msg) (*dns.Msg, error) {
	if err := prepareQUIC(query); err != nil {
		return nil, err
	}
	address, err := netip.ParseAddrPort(endpoint)
	if err != nil {
		return nil, err
	}
	packet, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}
	defer packet.Close()
	transport := &quic.Transport{Conn: packet}
	defer transport.Close()
	security := resolver.tlsConfig(upstream.Host)
	security.MinVersion, security.NextProtos = tls.VersionTLS13, []string{"doq"}
	connection, err := transport.Dial(ctx, net.UDPAddrFromAddrPort(address), security, &quic.Config{
		HandshakeIdleTimeout: resolver.attempt, MaxIdleTimeout: resolver.attempt,
		MaxIncomingStreams: -1, MaxIncomingUniStreams: -1,
		InitialStreamReceiveWindow: maxQUICWindow, MaxStreamReceiveWindow: maxQUICWindow,
		InitialConnectionReceiveWindow: maxQUICWindow, MaxConnectionReceiveWindow: maxQUICWindow,
	})
	if err != nil {
		return nil, err
	}
	defer connection.CloseWithError(0, "")
	stream, err := connection.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { stream.CancelRead(doqRequestCancelled); stream.CancelWrite(doqRequestCancelled) })
	defer stop()
	defer stream.CancelRead(doqRequestCancelled)
	defer stream.CancelWrite(doqRequestCancelled)
	deadline, _ := ctx.Deadline()
	if err := stream.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if err := writeFrame(stream, query); err != nil {
		return nil, err
	}
	if err := stream.Close(); err != nil {
		return nil, err
	}
	answer, err := readFrame(stream)
	if err != nil {
		connection.CloseWithError(doqProtocolError, "invalid DNS frame")
		return nil, err
	}
	var extra [1]byte
	if count, err := stream.Read(extra[:]); count != 0 || err != io.EOF {
		connection.CloseWithError(doqProtocolError, "invalid DNS stream ending")
		return nil, fmt.Errorf("DNS QUIC response must end after one frame: count %d, error %v", count, err)
	}
	if err := checkQUICOptions(answer); err != nil {
		connection.CloseWithError(doqProtocolError, "invalid DNS option")
		return nil, err
	}
	return answer, nil
}

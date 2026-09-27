package dnsproxy

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/miekg/dns"
)

const clientTimeout = 12 * time.Second
const maxTCPQueries = 64

func (runtime *proxy) dispatch(handle func()) bool {
	select {
	case runtime.handlers <- struct{}{}:
		runtime.workers.Add(1)
		go func() {
			defer runtime.workers.Done()
			defer func() { <-runtime.handlers }()
			handle()
		}()
		return true
	default:
		return false
	}
}

func (runtime *proxy) serveUDP(ctx context.Context, packet net.PacketConn) error {
	buffer := make([]byte, dns.MaxMsgSize)
	for {
		count, peer, err := packet.ReadFrom(buffer)
		if err != nil {
			return err
		}
		query := new(dns.Msg)
		if query.Unpack(buffer[:count]) != nil || query.Response {
			continue
		}
		runtime.dispatch(func() {
			answer := runtime.reply(ctx, query)
			limit := uint16(dns.MinMsgSize)
			if option := query.IsEdns0(); option != nil && option.UDPSize() > limit {
				limit = option.UDPSize()
			}
			answer.Truncate(int(limit))
			encoded, err := answer.Pack()
			if err == nil {
				_, _ = packet.WriteTo(encoded, peer) // A failed datagram has no session to recover.
			}
		})
	}
}

func (runtime *proxy) serveTCP(ctx context.Context, listener net.Listener) error {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return err
		}
		if !runtime.dispatch(func() { runtime.handleTCP(ctx, connection) }) {
			connection.Close()
		}
	}
}

func (runtime *proxy) handleTCP(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	stop := context.AfterFunc(ctx, func() { connection.Close() })
	defer stop()
	for count := 0; count < maxTCPQueries; count++ {
		if connection.SetDeadline(time.Now().Add(clientTimeout)) != nil {
			return
		}
		query, err := readFrame(connection)
		if err != nil || query.Response {
			return
		}
		if err := writeFrame(connection, runtime.reply(ctx, query)); err != nil {
			return
		}
	}
}

func (runtime *proxy) reply(ctx context.Context, query *dns.Msg) *dns.Msg {
	answer := new(dns.Msg)
	if validateQuery(query) != nil {
		answer.SetRcode(query, dns.RcodeFormatError)
		return answer
	}
	resolved, err := runtime.resolver.Exchange(ctx, query)
	if err != nil {
		answer.SetRcode(query, dns.RcodeServerFailure)
		answer.RecursionAvailable = true
		return answer
	}
	return resolved
}

// writeStatus uses the same bounded deadline as the readiness client.
func writeStatus(connection net.Conn, encoded []byte) error {
	if err := connection.SetDeadline(time.Now().Add(attemptTimeout)); err != nil {
		return err
	}
	count, err := connection.Write(encoded)
	if err == nil && count != len(encoded) {
		return io.ErrShortWrite
	}
	return err
}

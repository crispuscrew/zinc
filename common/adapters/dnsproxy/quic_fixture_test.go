package dnsproxy

import (
	"context"
	"crypto/tls"
	"io"
	"sync"
	"testing"

	"github.com/miekg/dns"
	quic "github.com/quic-go/quic-go"
)

func startQUIC(t *testing.T, security *tls.Config, respond func(*quic.Stream, *dns.Msg)) string {
	t.Helper()
	listener, err := quic.ListenAddr("127.0.0.1:0", security, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, err := listener.Accept(ctx)
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer connection.CloseWithError(0, "")
				stream, err := connection.AcceptStream(ctx)
				if err != nil {
					return
				}
				query, err := readFrame(stream)
				if err != nil {
					t.Error(err)
					return
				}
				if query.Id != 0 {
					t.Error("DoQ query ID must be zero")
				}
				encoded, err := query.Pack()
				if err != nil || len(encoded)%paddingBlock != 0 {
					t.Error("DoQ query missing block padding")
				}
				var extra [1]byte
				if count, err := stream.Read(extra[:]); count != 0 || err != io.EOF {
					t.Error("request missing FIN")
					return
				}
				respond(stream, query)
				select {
				case <-connection.Context().Done():
				case <-ctx.Done():
				}
			}()
		}
	}()
	t.Cleanup(func() { cancel(); listener.Close(); <-done; workers.Wait() })
	return listener.Addr().String()
}

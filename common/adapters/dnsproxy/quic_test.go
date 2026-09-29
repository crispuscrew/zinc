package dnsproxy

import (
	"context"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
	quic "github.com/quic-go/quic-go"
)

func TestQUICBootstrapIdentityAndLookup(t *testing.T) {
	security, roots := testCertificate(t)
	endpoint := startQUIC(t, security, func(stream *quic.Stream, query *dns.Msg) {
		if err := writeFrame(stream, testAnswer(query)); err != nil {
			t.Error(err)
		}
		if err := stream.Close(); err != nil {
			t.Error(err)
		}
	})
	resolver := testResolver(t, bootstrapMeta(schema.DNSQUIC, endpoint))
	resolver.roots = roots
	addresses, err := resolver.Lookup(context.Background(), "example.test")
	if err != nil || len(addresses) != 2 {
		t.Fatalf("lookup: %v %v", addresses, err)
	}
	resolver.roots = nil
	if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
		t.Fatal("accepted bad QUIC certificate")
	}
	resolver.roots = roots
	resolver.upstreams[0].Host = "wrong.test"
	if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
		t.Fatal("accepted wrong QUIC identity")
	}
}

func TestQUICFramingValidationAndTimeout(t *testing.T) {
	security, roots := testCertificate(t)
	for _, mode := range []string{"id", "question", "short", "incomplete", "extra", "no-fin", "timeout", "keepalive"} {
		t.Run(mode, func(t *testing.T) {
			endpoint := startQUIC(t, security, func(stream *quic.Stream, query *dns.Msg) {
				answer := testAnswer(query)
				switch mode {
				case "id":
					answer.Id = 42
				case "question":
					answer.Question[0].Name = "wrong.test."
				case "short":
					_, _ = stream.Write([]byte{0, 1, 0})
					_ = stream.Close()
					return
				case "incomplete":
					_, _ = stream.Write([]byte{0, 20, 0})
					_ = stream.Close()
					return
				case "timeout":
					return
				case "keepalive":
					answer.SetEdns0(dns.MaxMsgSize, false)
					answer.IsEdns0().Option = append(answer.IsEdns0().Option, &dns.EDNS0_TCP_KEEPALIVE{Code: dns.EDNS0TCPKEEPALIVE})
				}
				_ = writeFrame(stream, answer)
				if mode == "extra" {
					_, _ = stream.Write([]byte{1})
				}
				if mode != "no-fin" {
					_ = stream.Close()
				}
			})
			resolver := testResolver(t, bootstrapMeta(schema.DNSQUIC, endpoint))
			resolver.roots, resolver.attempt = roots, 200*time.Millisecond
			if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
				t.Fatalf("accepted %s", mode)
			}
		})
	}
}

func TestQUICCancellation(t *testing.T) {
	security, roots := testCertificate(t)
	arrived := make(chan struct{})
	endpoint := startQUIC(t, security, func(*quic.Stream, *dns.Msg) { close(arrived) })
	resolver := testResolver(t, bootstrapMeta(schema.DNSQUIC, endpoint))
	resolver.roots = roots
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := resolver.Exchange(ctx, testQuery()); done <- err }()
	select {
	case <-arrived:
	case <-time.After(time.Second):
		t.Fatal("query did not arrive")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled query succeeded")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("QUIC ignored cancellation")
	}
}

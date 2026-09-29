package dnsproxy

import (
	"context"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
	quic "github.com/quic-go/quic-go"
)

func TestQUICOptionsStayOnTheirOwnTransport(t *testing.T) {
	security, roots := testCertificate(t)
	endpoint := startQUIC(t, security, func(stream *quic.Stream, query *dns.Msg) {
		if err := checkQUICOptions(query); err != nil {
			t.Error(err)
		}
		answer := testAnswer(query)
		answer.Extra = query.Extra
		if err := writeFrame(stream, answer); err != nil {
			t.Error(err)
		}
		if err := stream.Close(); err != nil {
			t.Error(err)
		}
	})
	resolver := testResolver(t, bootstrapMeta(schema.DNSQUIC, endpoint))
	resolver.roots = roots
	query := testQuery()
	answer, err := resolver.Exchange(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if answer.IsEdns0() != nil || query.IsEdns0() != nil {
		t.Fatal("upstream-only EDNS leaked into non-EDNS client response or input")
	}
	query.SetEdns0(1232, true)
	query.IsEdns0().Option = []dns.EDNS0{&dns.EDNS0_TCP_KEEPALIVE{Code: dns.EDNS0TCPKEEPALIVE}}
	answer, err = resolver.Exchange(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if answer.IsEdns0() == nil || len(answer.IsEdns0().Option) != 0 {
		t.Fatal("DoQ response leaked transport options")
	}
	if len(query.IsEdns0().Option) != 1 || query.IsEdns0().Option[0].Option() != dns.EDNS0TCPKEEPALIVE {
		t.Fatal("mutated caller's query")
	}
}

func TestQUICRejectsConventionalDNSPort(t *testing.T) {
	if _, err := New(testMeta(schema.DNSQUIC, "127.0.0.1:53")); err == nil {
		t.Fatal("accepted prohibited DoQ port")
	}
}

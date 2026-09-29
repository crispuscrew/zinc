package dnsproxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

func privateDirectory(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func testCertificate(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"resolver.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	encoded, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(encoded)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{encoded}, PrivateKey: private}}, NextProtos: []string{"doq"}}, roots
}

func testMeta(protocol schema.DNSProtocol, endpoint string) schema.DNSMeta {
	return schema.DNSMeta{ResolversByPriority: []schema.DNSResolver{{Protocol: protocol, Endpoint: endpoint}}}
}

func bootstrapMeta(protocol schema.DNSProtocol, endpoint string) schema.DNSMeta {
	_, port, _ := net.SplitHostPort(endpoint)
	meta := testMeta(protocol, net.JoinHostPort("resolver.test", port))
	meta.ResolversByPriority[0].BootstrapIPs = []string{"127.0.0.1"}
	return meta
}

func testResolver(t *testing.T, meta schema.DNSMeta) *Resolver {
	t.Helper()
	resolver, err := New(meta)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func testQuery() *dns.Msg {
	return new(dns.Msg).SetQuestion("example.test.", dns.TypeA)
}

func testAnswer(query *dns.Msg) *dns.Msg {
	answer := new(dns.Msg).SetReply(query)
	header := dns.RR_Header{Name: query.Question[0].Name, Class: dns.ClassINET, Ttl: 60, Rrtype: query.Question[0].Qtype}
	switch query.Question[0].Qtype {
	case dns.TypeA:
		answer.Answer = []dns.RR{&dns.A{Hdr: header, A: net.ParseIP("192.0.2.7")}}
	case dns.TypeAAAA:
		answer.Answer = []dns.RR{&dns.AAAA{Hdr: header, AAAA: net.ParseIP("2001:db8::7")}}
	}
	return answer
}

func startClassic(t *testing.T, protocol schema.DNSProtocol, security *tls.Config, respond func(*dns.Msg) *dns.Msg) string {
	t.Helper()
	server := &dns.Server{Handler: dns.HandlerFunc(func(writer dns.ResponseWriter, query *dns.Msg) {
		if answer := respond(query); answer != nil {
			_ = writer.WriteMsg(answer)
		}
	})}
	var endpoint string
	var err error
	if protocol == schema.DNSUDP {
		server.PacketConn, err = net.ListenPacket("udp", "127.0.0.1:0")
		if err == nil {
			endpoint = server.PacketConn.LocalAddr().String()
		}
	} else {
		server.Listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			endpoint = server.Listener.Addr().String()
			if security != nil {
				server.Listener = tls.NewListener(server.Listener, security)
			}
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	server.NotifyStartedFunc = func() { close(started) }
	done := make(chan error, 1)
	go func() { done <- server.ActivateAndServe() }()
	<-started
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.ShutdownContext(ctx); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return endpoint
}

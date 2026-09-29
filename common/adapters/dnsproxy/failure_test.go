package dnsproxy

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

func TestCertificateAndTimeoutFailuresUseOnlyConfiguredFallback(t *testing.T) {
	security, _ := testCertificate(t)
	for _, mode := range []string{"certificate", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			fallback := startClassic(t, schema.DNSTCP, nil, func(query *dns.Msg) *dns.Msg {
				calls.Add(1)
				return testAnswer(query)
			})
			endpoint := startClassic(t, schema.DNSTLS, security, testAnswer)
			meta := bootstrapMeta(schema.DNSTLS, endpoint)
			if mode == "timeout" {
				endpoint = startClassic(t, schema.DNSUDP, nil, func(*dns.Msg) *dns.Msg { return nil })
				meta = testMeta(schema.DNSUDP, endpoint)
			}
			meta.ResolversByPriority = append(meta.ResolversByPriority, testMeta(schema.DNSTCP, fallback).ResolversByPriority...)
			resolver := testResolver(t, meta)
			resolver.attempt = 100 * time.Millisecond
			if _, err := resolver.Exchange(context.Background(), testQuery()); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatal("configured fallback was not used")
			}
		})
	}
}

func TestTCPRejectsIncompleteResponseFrame(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		if _, err := readFrame(connection); err == nil {
			_, _ = connection.Write([]byte{0, 64, 0})
		}
	}()
	resolver := testResolver(t, testMeta(schema.DNSTCP, listener.Addr().String()))
	if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
		t.Fatal("accepted incomplete TCP response frame")
	}
	<-done
}

func TestTCPAndTLSReadCancellation(t *testing.T) {
	security, roots := testCertificate(t)
	for _, protocol := range []schema.DNSProtocol{schema.DNSTCP, schema.DNSTLS} {
		t.Run(string(protocol), func(t *testing.T) {
			arrived := make(chan struct{})
			serverTLS := security
			if protocol == schema.DNSTCP {
				serverTLS = nil
			}
			endpoint := startClassic(t, protocol, serverTLS, func(*dns.Msg) *dns.Msg { close(arrived); return nil })
			resolver := testResolver(t, bootstrapMeta(protocol, endpoint))
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
				t.Fatal("cancellation did not interrupt read")
			}
		})
	}
}

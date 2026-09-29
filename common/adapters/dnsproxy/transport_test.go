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

func TestClassicTransportsBootstrapAndCertificate(t *testing.T) {
	security, roots := testCertificate(t)
	for _, protocol := range []schema.DNSProtocol{schema.DNSUDP, schema.DNSTCP, schema.DNSTLS} {
		t.Run(string(protocol), func(t *testing.T) {
			serverTLS := security
			if protocol != schema.DNSTLS {
				serverTLS = nil
			}
			endpoint := startClassic(t, protocol, serverTLS, testAnswer)
			meta := bootstrapMeta(protocol, endpoint)
			resolver := testResolver(t, meta)
			resolver.roots = roots
			addresses, err := resolver.Lookup(context.Background(), "example.test")
			if err != nil || len(addresses) != 2 {
				t.Fatalf("lookup: %v %v", addresses, err)
			}
			if protocol == schema.DNSTLS {
				resolver.roots = nil
				if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
					t.Fatal("accepted untrusted certificate")
				}
				resolver.roots = roots
				resolver.upstreams[0].Host = "wrong.test"
				if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
					t.Fatal("accepted wrong TLS identity")
				}
			}
		})
	}
}

func TestPriorityValidationAndNXDOMAIN(t *testing.T) {
	mutations := map[string]func(*dns.Msg){
		"id":        func(answer *dns.Msg) { answer.Id++ },
		"qr":        func(answer *dns.Msg) { answer.Response = false },
		"opcode":    func(answer *dns.Msg) { answer.Opcode = dns.OpcodeStatus },
		"name":      func(answer *dns.Msg) { answer.Question[0].Name = "wrong.test." },
		"type":      func(answer *dns.Msg) { answer.Question[0].Qtype = dns.TypeAAAA },
		"class":     func(answer *dns.Msg) { answer.Question[0].Qclass = dns.ClassCHAOS },
		"questions": func(answer *dns.Msg) { answer.Question = nil },
		"servfail":  func(answer *dns.Msg) { answer.Rcode = dns.RcodeServerFailure },
		"refused":   func(answer *dns.Msg) { answer.Rcode = dns.RcodeRefused },
		"truncated": func(answer *dns.Msg) { answer.Truncated = true },
		"nxdomain":  func(answer *dns.Msg) { answer.Rcode, answer.Answer = dns.RcodeNameError, nil },
		"success":   func(*dns.Msg) {},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var fallback atomic.Int32
			first := startClassic(t, schema.DNSUDP, nil, func(query *dns.Msg) *dns.Msg {
				answer := testAnswer(query)
				mutate(answer)
				return answer
			})
			second := startClassic(t, schema.DNSTCP, nil, func(query *dns.Msg) *dns.Msg {
				fallback.Add(1)
				return testAnswer(query)
			})
			meta := testMeta(schema.DNSUDP, first)
			meta.ResolversByPriority = append(meta.ResolversByPriority, testMeta(schema.DNSTCP, second).ResolversByPriority...)
			query := testQuery()
			answer, err := testResolver(t, meta).Exchange(context.Background(), query)
			if err != nil || answer.Id != query.Id {
				t.Fatalf("exchange: %v %v", answer, err)
			}
			terminal := name == "nxdomain" || name == "success"
			if (fallback.Load() == 0) != terminal {
				t.Fatalf("fallback count %d", fallback.Load())
			}
		})
	}
}

func TestCancellationTimeoutAndNoImplicitTCP(t *testing.T) {
	endpoint := startClassic(t, schema.DNSUDP, nil, func(*dns.Msg) *dns.Msg { return nil })
	resolver := testResolver(t, testMeta(schema.DNSUDP, endpoint))
	resolver.attempt = 30 * time.Millisecond
	started := time.Now()
	if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
		t.Fatal("timeout accepted")
	}
	if time.Since(started) > time.Second {
		t.Fatal("timeout ignored")
	}
	resolver.attempt = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	started = time.Now()
	if _, err := resolver.Exchange(ctx, testQuery()); err == nil {
		t.Fatal("cancellation accepted")
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("cancellation did not interrupt read")
	}
	truncated := startClassic(t, schema.DNSUDP, nil, func(query *dns.Msg) *dns.Msg {
		answer := testAnswer(query)
		answer.Truncated = true
		return answer
	})
	tcp, err := net.Listen("tcp", truncated)
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	resolver = testResolver(t, testMeta(schema.DNSUDP, truncated))
	if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
		t.Fatal("truncation accepted")
	}
	if err := tcp.(*net.TCPListener).SetDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if connection, err := tcp.Accept(); err == nil {
		connection.Close()
		t.Fatal("implicit TCP fallback")
	}
}

func TestNoAmbientResolver(t *testing.T) {
	for _, protocol := range []schema.DNSProtocol{schema.DNSUDP, schema.DNSTCP, schema.DNSTLS, schema.DNSHTTPS, schema.DNSQUIC} {
		if _, err := New(testMeta(protocol, "resolver.test")); err == nil {
			t.Fatalf("%s missing bootstrap accepted", protocol)
		}
	}
	if _, err := New(schema.DNSMeta{}); err == nil {
		t.Fatal("empty config accepted")
	}
}

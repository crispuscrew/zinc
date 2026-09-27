package dnsproxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

func TestHTTPSIdentityBootstrapAndNoAmbientProxy(t *testing.T) {
	security, roots := testCertificate(t)
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.TLS.ServerName != "resolver.test" || !strings.HasPrefix(request.Host, "resolver.test:") || request.URL.Path != "/custom-dns" {
			t.Error("lost endpoint TLS/HTTP identity or custom path")
		}
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/dns-message" {
			t.Error("invalid DoH request")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		query := new(dns.Msg)
		if err := query.Unpack(body); err != nil {
			t.Error(err)
			return
		}
		encoded, err := testAnswer(query).Pack()
		if err != nil {
			t.Error(err)
			return
		}
		writer.Header().Set("Content-Type", "application/dns-message")
		_, _ = writer.Write(encoded)
	}))
	server.TLS = security
	server.StartTLS()
	defer server.Close()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	meta := bootstrapMeta(schema.DNSHTTPS, server.Listener.Addr().String())
	meta.ResolversByPriority[0].Path = "/custom-dns"
	resolver := testResolver(t, meta)
	resolver.roots = roots
	if _, err := resolver.Exchange(context.Background(), testQuery()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests: %d", requests.Load())
	}
	resolver.roots = nil
	if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
		t.Fatal("accepted bad HTTPS certificate")
	}
}

func TestHTTPSRejectsRedirectsMediaStatusOversizeAndMismatch(t *testing.T) {
	security, roots := testCertificate(t)
	for _, mode := range []string{"redirect", "media", "status", "oversize", "id", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			var redirected atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/redirected" {
					redirected.Add(1)
				}
				writer.Header().Set("Content-Type", "application/dns-message")
				switch mode {
				case "redirect":
					http.Redirect(writer, request, "/redirected", http.StatusTemporaryRedirect)
				case "media":
					writer.Header().Set("Content-Type", "text/plain")
				case "status":
					writer.WriteHeader(http.StatusServiceUnavailable)
				case "oversize":
					_, _ = writer.Write(make([]byte, dns.MaxMsgSize+1))
				case "timeout":
					_, _ = io.Copy(io.Discard, request.Body)
					<-request.Context().Done()
				case "id":
					body, _ := io.ReadAll(request.Body)
					query := new(dns.Msg)
					if query.Unpack(body) != nil {
						return
					}
					answer := testAnswer(query)
					answer.Id++
					encoded, _ := answer.Pack()
					_, _ = writer.Write(encoded)
				}
			}))
			server.TLS = security.Clone()
			server.StartTLS()
			defer server.Close()
			resolver := testResolver(t, bootstrapMeta(schema.DNSHTTPS, server.Listener.Addr().String()))
			resolver.roots, resolver.attempt = roots, 100*time.Millisecond
			if _, err := resolver.Exchange(context.Background(), testQuery()); err == nil {
				t.Fatalf("accepted %s", mode)
			}
			if redirected.Load() != 0 {
				t.Fatal("followed redirect")
			}
		})
	}
}

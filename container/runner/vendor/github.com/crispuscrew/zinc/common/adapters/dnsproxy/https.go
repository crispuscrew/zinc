package dnsproxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/miekg/dns"
)

func (resolver *Resolver) https(ctx context.Context, upstream domain.Upstream, endpoint string, query *dns.Msg) (*dns.Msg, error) {
	encoded, err := query.Pack()
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy: nil, TLSClientConfig: resolver.tlsConfig(upstream.Host),
		DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 8192,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", endpoint)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return fmt.Errorf("DNS HTTP redirects are forbidden")
	}}
	url := "https://" + net.JoinHostPort(upstream.Host, strconv.Itoa(upstream.Port)) + upstream.Path
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/dns-message")
	request.Header.Set("Accept", "application/dns-message")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || err != nil || media != "application/dns-message" {
		return nil, fmt.Errorf("invalid DNS HTTP response: status %d, media %q", response.StatusCode, media)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, dns.MaxMsgSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > dns.MaxMsgSize {
		return nil, fmt.Errorf("DNS HTTP response too large")
	}
	answer := new(dns.Msg)
	if err := answer.Unpack(body); err != nil {
		return nil, err
	}
	return answer, nil
}

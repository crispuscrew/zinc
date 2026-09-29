package dnsproxy

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

func startProxy(t *testing.T) (*proxy, schema.DNSMeta, string, context.CancelFunc, <-chan error) {
	t.Helper()
	endpoint := startClassic(t, schema.DNSUDP, nil, testAnswer)
	meta := testMeta(schema.DNSUDP, endpoint)
	socket := filepath.Join(privateDirectory(t), "control.sock")
	runtime, err := bind(meta, []string{"127.0.0.1:0"}, socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	terminated := make(chan struct{})
	go func() { defer close(terminated); done <- runtime.run(ctx) }()
	t.Cleanup(func() { cancel(); <-terminated })
	deadline := time.Now().Add(time.Second)
	for CheckReady(socket, meta, runtime.status.Addresses) != nil {
		if time.Now().After(deadline) {
			t.Fatal("proxy did not become ready")
		}
		time.Sleep(time.Millisecond)
	}
	return runtime, meta, socket, cancel, done
}

func TestProxyUDPAndTCPReadinessAndCleanup(t *testing.T) {
	runtime, meta, socket, cancel, done := startProxy(t)
	for _, transport := range []string{"udp", "tcp"} {
		query := testQuery()
		answer, _, err := (&dns.Client{Net: transport, Timeout: time.Second}).Exchange(query, runtime.status.Addresses[0])
		if err != nil || answer.Id != query.Id || len(answer.Answer) != 1 {
			t.Fatalf("%s: %v %v", transport, answer, err)
		}
	}
	wrong := testMeta(schema.DNSUDP, "127.0.0.1:1")
	if CheckReady(socket, wrong, runtime.status.Addresses) == nil {
		t.Fatal("accepted different config")
	}
	if CheckReady(socket, meta, []string{"127.0.0.1:1"}) == nil {
		t.Fatal("accepted different listeners")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked")
	}
	if _, err := os.Lstat(socket); !os.IsNotExist(err) {
		t.Fatalf("socket not removed: %v", err)
	}
	if CheckReady(socket, meta, runtime.status.Addresses) == nil {
		t.Fatal("stopped proxy is ready")
	}
	listener, err := net.Listen("tcp", runtime.status.Addresses[0])
	if err != nil {
		t.Fatal("TCP listener leaked", err)
	}
	listener.Close()
	packet, err := net.ListenPacket("udp", runtime.status.Addresses[0])
	if err != nil {
		t.Fatal("UDP listener leaked", err)
	}
	packet.Close()
}

func TestProxyBoundedHandlersAndShutdown(t *testing.T) {
	runtime, _, _, cancel, done := startProxy(t)
	var clients []net.Conn
	defer func() {
		for _, client := range clients {
			client.Close()
		}
	}()
	for index := 0; index < maxHandlers+4; index++ {
		client, err := net.DialTimeout("tcp", runtime.status.Addresses[0], time.Second)
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, client)
	}
	deadline := time.Now().Add(time.Second)
	for len(runtime.handlers) != maxHandlers {
		if time.Now().After(deadline) {
			t.Fatal("handler bound not reached")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("idle TCP clients blocked shutdown")
	}
	if len(runtime.handlers) != 0 {
		t.Fatal("handlers leaked")
	}
}

func TestProxyPartialBindRollsBack(t *testing.T) {
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer packet.Close()
	socket := filepath.Join(privateDirectory(t), "control.sock")
	meta := testMeta(schema.DNSUDP, "127.0.0.1:1")
	if _, err := bind(meta, []string{packet.LocalAddr().String()}, socket); err == nil {
		t.Fatal("accepted unavailable UDP listener")
	}
	if _, err := os.Lstat(socket); !os.IsNotExist(err) {
		t.Fatal("published readiness before binding all listeners")
	}
	listener, err := net.Listen("tcp", packet.LocalAddr().String())
	if err != nil {
		t.Fatal("partial TCP bind leaked", err)
	}
	listener.Close()
}

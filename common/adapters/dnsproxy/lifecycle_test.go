package dnsproxy

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

func TestProxyListenerFailureWithdrawsReadiness(t *testing.T) {
	runtime, meta, socket, _, done := startProxy(t)
	if err := runtime.packets[0].Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("listener failure was hidden")
		}
	case <-time.After(time.Second):
		t.Fatal("listener failure did not stop proxy")
	}
	if CheckReady(socket, meta, runtime.status.Addresses) == nil {
		t.Fatal("failed listener remained ready")
	}
}

func TestProxyDropsResponsePackets(t *testing.T) {
	runtime, _, _, cancel, done := startProxy(t)
	for _, transport := range []string{"udp", "tcp"} {
		query := testAnswer(testQuery())
		client := &dns.Client{Net: transport, Timeout: 30 * time.Millisecond}
		if _, _, err := client.Exchange(query, runtime.status.Addresses[0]); err == nil {
			t.Fatalf("%s reflected a response packet", transport)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServeAlreadyCancelledDoesNotBind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(privateDirectory(t), "control.sock")
	meta := testMeta(schema.DNSUDP, "127.0.0.1:1")
	if err := Serve(ctx, meta, []string{"127.0.0.1:0"}, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("cancelled Serve created a socket")
	}
}

func TestListenerAddressesAreNumericUnicastAndUnique(t *testing.T) {
	for _, values := range [][]string{nil, {"localhost:53"}, {"0.0.0.0:53"}, {"[::]:53"}, {"224.0.0.1:53"}, {"255.255.255.255:53"}, {"127.0.0.1", "127.0.0.1:53"}} {
		if _, err := listenerAddresses(values); err == nil {
			t.Fatalf("accepted %v", values)
		}
	}
}

func TestProxyClosesIncompleteTCPFrames(t *testing.T) {
	runtime, _, _, cancel, done := startProxy(t)
	connection, err := net.Dial("tcp", runtime.status.Addresses[0])
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte{0, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var buffer [1]byte
	if _, err := connection.Read(buffer[:]); !errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("malformed DNS frame did not close connection: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

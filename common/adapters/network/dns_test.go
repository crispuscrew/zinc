package network

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/adapters/dnsproxy"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestDNSRequiresAuthenticatedLiveConfiguration(t *testing.T) {
	cfg := schema.AppConfig{NetworkMeta: schema.NetworkMeta{DNS: schema.DNSMeta{
		ResolversByPriority: []schema.DNSResolver{{Protocol: schema.DNSUDP, Endpoint: "192.0.2.53"}},
	}}}
	manifest := Manifest{}
	if err := ReadyDNS(cfg, manifest); err == nil || !strings.Contains(err.Error(), "dns_control_socket") {
		t.Fatalf("unverified DNS accepted: %v", err)
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest.DNSControlSocket = filepath.Join(directory, "control.sock")
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		finished <- dnsproxy.Serve(ctx, cfg.NetworkMeta.DNS, []string{"127.0.0.1:0"}, manifest.DNSControlSocket)
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-finished; err != nil {
			t.Errorf("proxy cleanup: %v", err)
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		connection, err := net.DialTimeout("unix", manifest.DNSControlSocket, 100*time.Millisecond)
		if err == nil {
			_ = connection.SetDeadline(time.Now().Add(time.Second))
			var status struct {
				Addresses []string `json:"addresses"`
				Ready     bool     `json:"ready"`
			}
			err = json.NewDecoder(connection).Decode(&status)
			connection.Close()
			if err == nil && status.Ready {
				manifest.DNSProxyAddresses = status.Addresses
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("proxy never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := ReadyDNS(cfg, manifest); err != nil {
		t.Fatal(err)
	}
	cfg.NetworkMeta.DNS.ResolversByPriority[0].Protocol = schema.DNSTCP
	if err := ReadyDNS(cfg, manifest); err == nil {
		t.Fatal("different upstream policy was accepted")
	}
	if err := ReadyDNS(schema.AppConfig{}, manifest); err == nil {
		t.Fatal("undeclared proxy was accepted")
	}
	if err := ReadyDNS(schema.AppConfig{}, Manifest{}); err != nil {
		t.Fatal(err)
	}
}

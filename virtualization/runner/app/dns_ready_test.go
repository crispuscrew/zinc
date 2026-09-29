package app

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
)

func readyProxy(t *testing.T, manifest *provision.Manifest) {
	t.Helper()
	directory, err := os.MkdirTemp("", "zinc-dns-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	manifest.DNSControlSocket = filepath.Join(directory, "status.sock")
	listener, err := net.Listen("unix", manifest.DNSControlSocket)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(manifest.DNSControlSocket, 0o600); err != nil {
		listener.Close()
		t.Fatal(err)
	}
	addresses := make([]string, len(manifest.DNSProxyAddresses))
	for index, address := range manifest.DNSProxyAddresses {
		addresses[index] = net.JoinHostPort(address, "53")
	}
	status := map[string]any{"version": 1, "ready": true, "digest": manifest.DNSConfigDigest, "addresses": addresses}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			_ = json.NewEncoder(connection).Encode(status)
			connection.Close()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
}

package dnsproxy

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIValidation(t *testing.T) {
	if Run(context.Background(), nil) == nil {
		t.Fatal("accepted missing flags")
	}
	if Run(context.Background(), []string{"--unknown"}) == nil {
		t.Fatal("accepted unknown flag")
	}
	path := filepath.Join(t.TempDir(), "dns.json")
	if err := os.WriteFile(path, []byte(`{"Unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if Run(context.Background(), []string{"--config", path, "--listen", "127.0.0.1:0", "--control-socket", path + ".sock"}) == nil {
		t.Fatal("accepted unknown config field")
	}
	if err := os.WriteFile(path, []byte(`{}`+strings.Repeat(" ", 128*1024)), 0600); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"--config", path, "--listen", "127.0.0.1:0", "--control-socket", path + ".sock"})
	if err == nil || !strings.Contains(err.Error(), "oversized") {
		t.Fatalf("oversized whitespace input not rejected: %v", err)
	}
}

func TestPeerCredentials(t *testing.T) {
	if !trustedUID(0) || !trustedUID(uint32(os.Geteuid())) || trustedUID(uint32(os.Geteuid())+1) {
		t.Fatal("peer UID policy must accept only root/self")
	}
	path := filepath.Join(privateDirectory(t), "control.sock")
	control, err := listenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	defer control.close()
	connection, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := checkPeer(connection); err != nil {
		t.Fatal(err)
	}
}

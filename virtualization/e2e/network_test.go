//go:build e2e

package e2e

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func waitForSSH(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
		if err == nil {
			connection.SetReadDeadline(time.Now().Add(2 * time.Second))
			banner, err := bufio.NewReader(connection).ReadString('\n')
			connection.Close()
			if err == nil && strings.HasPrefix(banner, "SSH-") {
				return true
			}
		}
		time.Sleep(time.Second)
	}
	return false
}

func assertLoopbackOnly(t *testing.T, port int) {
	t.Helper()
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range addresses {
		network, valid := address.(*net.IPNet)
		if !valid || network.IP.IsLoopback() || network.IP.IsLinkLocalUnicast() {
			continue
		}
		connection, err := net.DialTimeout("tcp", net.JoinHostPort(network.IP.String(), strconv.Itoa(port)), time.Second)
		if err == nil {
			connection.Close()
			t.Errorf("forward reachable on non-loopback address %s", network.IP)
		}
	}
}

func assertNothingLeftBehind(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir("/proc")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range entries {
			if _, err := strconv.Atoi(entry.Name()); err != nil {
				continue
			}
			raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
			if err != nil {
				continue
			}
			argv := strings.Split(string(raw), "\x00")
			if len(argv) > 0 && strings.Contains(string(raw), name) &&
				(strings.Contains(argv[0], "qemu-system") || strings.Contains(argv[0], "pasta") || strings.Contains(argv[0], "swtpm")) {
				found = true
			}
		}
		if !found {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("guest or helper survived stop")
}

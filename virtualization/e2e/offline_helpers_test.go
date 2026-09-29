//go:build e2e

package e2e

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func guestPID(env environment, name string) int {
	body, err := os.ReadFile(filepath.Join(env.runtime, "zinc", "vm", name+".pid"))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(body)))
	return pid
}

func readGuestPID(check *testing.T, env environment, name string) int {
	check.Helper()
	pid := guestPID(env, name)
	if pid <= 1 {
		check.Fatal("runner did not record a host guest PID")
	}
	return pid
}

func verifyGuestProcess(check *testing.T, pid int, overlay string) {
	check.Helper()
	body, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || !strings.Contains(string(body), "qemu-system-x86_64") || !strings.Contains(string(body), overlay) {
		check.Fatal("refusing to signal a process not belonging to this private guest")
	}
}

func assertReadOnlyBlock(check *testing.T, socket string) {
	check.Helper()
	connection, err := net.DialTimeout("unix", socket, 3*time.Second)
	if err != nil {
		check.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(5 * time.Second))
	decoder, encoder := json.NewDecoder(connection), json.NewEncoder(connection)
	var greeting map[string]any
	if err := decoder.Decode(&greeting); err != nil {
		check.Fatal(err)
	}
	query := func(command string) json.RawMessage {
		if err := encoder.Encode(map[string]string{"execute": command}); err != nil {
			check.Fatal(err)
		}
		for {
			var response struct {
				Return json.RawMessage `json:"return"`
				Error  json.RawMessage `json:"error"`
				Event  string          `json:"event"`
			}
			if err := decoder.Decode(&response); err != nil {
				check.Fatal(err)
			}
			if response.Error != nil {
				check.Fatalf("QMP error: %s", response.Error)
			}
			if response.Return != nil {
				return response.Return
			}
		}
	}
	query("qmp_capabilities")
	var blocks []struct {
		Inserted struct {
			ReadOnly bool `json:"ro"`
		} `json:"inserted"`
	}
	if err := json.Unmarshal(query("query-block"), &blocks); err != nil {
		check.Fatal(err)
	}
	if len(blocks) != 1 || !blocks[0].Inserted.ReadOnly {
		check.Fatal("root block device is not read-only", blocks)
	}
}

//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func (env environment) busAttribution(t *testing.T, proxy string) {
	table := must(t, env.runner, "bus")
	var reported string
	for _, line := range strings.Split(table, "\n") {
		if fields := strings.Fields(line); len(fields) >= 3 && fields[0] == "busapp" {
			reported = fields[1]
		}
	}
	actual := must(t, "podman", "inspect", "--format", "{{.State.Pid}}", proxy)
	if reported == "" || strings.TrimSpace(actual) != reported {
		t.Fatalf("attribution mismatch: %s actual %s", table, actual)
	}
	raw := must(t, env.runner, "where", "busapp", "--json")
	var report struct {
		Address string `json:"address"`
		Bus     *struct {
			Socket string `json:"socket"`
			Proxy  string `json:"proxy"`
		} `json:"bus"`
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatal(err)
	}
	if report.Address != "busapp" || report.Bus == nil || report.Bus.Proxy != proxy {
		t.Fatal(raw)
	}
	if _, err := os.Stat(report.Bus.Socket); err != nil {
		t.Fatal(err)
	}
	t.Run("host_bus_resolution", func(t *testing.T) {
		for _, needed := range []string{"gdbus", "busctl"} {
			if _, err := exec.LookPath(needed); err != nil {
				t.Skipf("no %s", needed)
			}
		}
		client := exec.Command("gdbus", "wait", "--address", "unix:path="+report.Bus.Socket, "--timeout", "20", "org.example.NeverAppears")
		if err := client.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { client.Process.Kill(); client.Wait() }()
		if !waitFor(func() bool {
			names, _ := tool("busctl", "--user", "list", "--unique")
			for _, line := range strings.Split(names, "\n") {
				if fields := strings.Fields(line); len(fields) >= 2 && fields[1] == reported {
					return true
				}
			}
			return false
		}) {
			t.Fatal("host bus connection does not resolve to proxy pid")
		}
	})
}

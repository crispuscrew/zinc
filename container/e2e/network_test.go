//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func (env environment) missingNetwork(t *testing.T) {
	t.Setenv("ZINC_NETWORK_MANIFEST_DIR", t.TempDir())
	output, err := tool(env.runner, "run", "producer", "--exec")
	if err == nil || !strings.Contains(output, "preprovisioned") {
		t.Fatalf("missing topology was not refused: %s %v", output, err)
	}
	if env.running("producer") {
		t.Fatal("producer started without provisioned topology")
	}
}

func (env environment) network(t *testing.T) {
	if os.Getenv("ZINC_E2E_NO_NET") != "" {
		t.Skip("ZINC_E2E_NO_NET set")
	}
	manifest := os.Getenv("ZINC_E2E_NETWORK_MANIFEST_DIR")
	if manifest == "" {
		t.Skip("set ZINC_E2E_NETWORK_MANIFEST_DIR to owner-provisioned producer/consumer manifests; topology is never created by tests")
	}
	t.Setenv("ZINC_NETWORK_MANIFEST_DIR", manifest)
	defer tool(env.runner, "stop", "consumer")
	defer tool(env.runner, "stop", "producer")
	env.start(t, "producer")
	env.start(t, "consumer")
	verdict := env.logs(t, "consumer", "PROBE ")
	for _, want := range []string{"5432=open", "9999=closed"} {
		if !strings.Contains(verdict, want) {
			t.Errorf("missing %s in %s", want, verdict)
		}
	}
	env.start(t, "sleeper")
	defer tool(env.runner, "stop", "sleeper")
	listing := must(t, env.runner, "net")
	for _, want := range []string{"producer", "filtered", "producer-pod", "sleeper", "isolated"} {
		if !strings.Contains(listing, want) {
			t.Errorf("missing %s in %s", want, listing)
		}
	}
	var accepted, denied uint64
	for _, name := range []string{"producer", "consumer"} {
		raw := must(t, env.runner, "net", name, "--json")
		var report struct {
			Posture  string `json:"posture"`
			Netns    string `json:"netns"`
			Counters []struct {
				Label   string `json:"label"`
				Packets uint64 `json:"packets"`
			} `json:"counters"`
		}
		if err := json.Unmarshal([]byte(raw), &report); err != nil {
			t.Fatalf("%v: %s", err, raw)
		}
		if report.Posture != "filtered" || report.Netns != name+"-pod" {
			t.Fatal(raw)
		}
		for _, counter := range report.Counters {
			if strings.Contains(counter.Label, "producer rule[0]") {
				accepted += counter.Packets
			}
			if strings.Contains(counter.Label, "default") {
				denied += counter.Packets
			}
		}
	}
	// Reciprocal policy may drop the forbidden packet at the consumer before it arrives.
	if accepted == 0 || denied == 0 {
		t.Fatalf("expected accepted and refused traffic, got %d/%d", accepted, denied)
	}
	isolated := must(t, env.runner, "net", "sleeper")
	if !strings.Contains(isolated, "isolated") || strings.Contains(isolated, "PACKETS") {
		t.Fatal(isolated)
	}
}

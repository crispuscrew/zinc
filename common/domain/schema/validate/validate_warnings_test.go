package validate

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestFingerprintConflictsAreAdvisories(test *testing.T) {
	cfg := baseCfg()
	cfg.MinimizeFingerprint = true
	if warnings := Warnings(cfg); len(warnings) != 0 {
		test.Fatalf("minimization without conflicts: %v", warnings)
	}
	cfg.DisplayMeta = schema.DisplayMeta{Vulkan: true, DisplayWidth: 1920, DisplayHeight: 1080}
	cfg.InternalUserMeta.KeepUserID = true
	cfg.HostTheme = true
	cfg.RunnerFlags = []string{"--hostname=stable"}
	cfg.CreatorFlags = []string{"--build-arg=HOST=stable"}
	cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "wan", MacAddress: "02:11:22:33:44:55"}}
	cfg.AudioMeta.Playback.PipeWireDevices = []string{"alsa_output.card"}
	if err := Validate(cfg); err != nil {
		test.Fatalf("fingerprint conflicts are warnings: %v", err)
	}
	joined := strings.Join(Warnings(cfg), "\n")
	for _, field := range []string{"DisplayMeta.Vulkan", "DisplayWidth/DisplayHeight", "KeepUserID", "HostTheme", "CreatorFlags", "RunnerFlags", "MacAddress", "AudioMeta device selectors"} {
		if !strings.Contains(joined, "MinimizeFingerprint: "+field) && !strings.Contains(joined, "."+field) {
			test.Errorf("missing conflict for %s: %s", field, joined)
		}
	}
}

func TestNetworkWarningsDoNotInventConnectivity(test *testing.T) {
	deny := outboundRule()
	deny.AllowAllExcept = true
	cfg := networkCfg(deny)
	cfg.NetworkMeta.DNS.ResolversByPriority = nil
	joined := strings.Join(Warnings(cfg), "\n")
	if !strings.Contains(joined, "no explicit resolvers") || strings.Contains(joined, "allow-all") {
		test.Fatalf("deny match must not imply DNS allowance: %s", joined)
	}
	cfg.NetworkMeta.Interfaces = nil
	if joined = strings.Join(Warnings(cfg), "\n"); !strings.Contains(joined, "no external NIC") {
		test.Fatal(joined)
	}
	rule := outboundRule()
	rule.From, rule.To = rule.To, rule.From
	if joined = strings.Join(Warnings(networkCfg(rule)), "\n"); !strings.Contains(joined, "inbound") {
		test.Fatal(joined)
	}
	rule.AllowAllExcept = true
	if joined = strings.Join(Warnings(networkCfg(rule)), "\n"); strings.Contains(joined, "inbound") {
		test.Fatal(joined)
	}
}

func TestVMVulkanSandboxWarning(test *testing.T) {
	cfg := baseVM()
	cfg.DisplayMeta.Vulkan = true
	if joined := strings.Join(Warnings(cfg), "\n"); !strings.Contains(joined, "seccomp sandbox") {
		test.Fatal(joined)
	}
	if warnings := Warnings(baseVM()); len(warnings) != 0 {
		test.Fatal(warnings)
	}
}

package compose

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestExportBaselineAndMovedFields(t *testing.T) {
	cfg := containerApp("app")
	cfg.LauncherMeta = schema.LauncherMeta{Description: "description", Icon: "icon", Group: "group"}
	cfg.StartConditions.EntrypointEnv = map[string]string{"NAME": "value with spaces"}
	cfg.StartConditions.ReadOnlyRootfs = true
	cfg.StopConditions.Autorestart = true
	cfg.StartConditions.DependsOn = []string{"db"}
	service, _ := exportApp(t, cfg)
	if !slices.Contains(service.CapDrop, "ALL") || !slices.Contains(service.SecurityOpt, "no-new-privileges:true") || service.NetworkMode != "none" {
		t.Fatal(service)
	}
	if service.Restart != "on-failure" || !service.ReadOnly || service.Environment["NAME"] != "value with spaces" || service.Labels["zinc.group"] != "group" {
		t.Fatal(service)
	}
	if service.DependsOn["db"].Condition != ConditionStarted {
		t.Fatal("invented readiness")
	}
}

func TestExportRawAndAttachedLosses(t *testing.T) {
	cfg := containerApp("app")
	cfg.RunnerFlags = []string{"--cap-add", "NET_RAW"}
	cfg.StartConditions.AttachedEnv = map[string]string{"SESSION": "value"}
	cfg.MinimizeFingerprint = true
	cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSHTTPS, Endpoint: "dns.example", Path: "/dns-query"}}
	service, notes := exportApp(t, cfg)
	for _, want := range []string{"RunnerFlags", "AttachedEnv", "MinimizeFingerprint", "DNS.ResolversByPriority"} {
		if !strings.Contains(strings.Join(notes, "\n"), want) {
			t.Errorf("missing loss %s: %v", want, notes)
		}
	}
	if len(service.CapAdd)+len(service.DNS) != 0 {
		t.Fatal("raw flags or encrypted DNS silently reinterpreted")
	}
}

func TestExportVMRefused(t *testing.T) {
	cfg := containerApp("guest")
	cfg.Type = schema.ZincVirtualization
	if _, _, err := FromApp(cfg); err == nil {
		t.Fatal("exported VM as container")
	}
}

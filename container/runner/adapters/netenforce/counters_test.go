package netenforce

import (
	"errors"
	"strings"
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func TestCountersUsesPinnedNamespaceWithoutChangingFirewall(t *testing.T) {
	cfg, manifest := configured()
	command, active := adapter(manifest).Counters(cfg, options.HostOptions{})
	joined := strings.Join(command.Args, " ")
	if !active || !strings.Contains(joined, "net:[123]") || !strings.Contains(joined, "nft -j list table inet zinc") || strings.Contains(joined, "nft -f") {
		t.Fatal("counter path differs from enforcement")
	}
	if flagValue(command.Args, "--network") != "ns:/proc/321/ns/net" {
		t.Fatal("counters did not join the observed app process namespace")
	}
	cfg.NetworkMeta.Interfaces = nil
	if _, active := adapter(manifest).Counters(cfg, options.HostOptions{}); active {
		t.Fatal("isolated app has counters")
	}
}

func TestCountersRefuseMissingProcess(test *testing.T) {
	for _, failure := range []error{nil, errors.New("inspect denied")} {
		cfg, manifest := configured()
		enforcer := adapter(manifest)
		enforcer.ProcessID = func(string) (int, error) { return 0, failure }
		command, active := enforcer.Counters(cfg, options.HostOptions{})
		if !active || flagValue(command.Args, "--network") != "none" ||
			!strings.Contains(command.Args[len(command.Args)-1], "exit 1") {
			test.Fatal("missing process produced misleading counters")
		}
	}
}

func TestCountersFreezeObservedProcess(test *testing.T) {
	cfg, manifest := configured()
	enforcer := adapter(manifest)
	calls := 0
	enforcer.ProcessID = func(name string) (int, error) {
		if name != cfg.AppNameID {
			test.Fatalf("inspected wrong app: %s", name)
		}
		calls++
		return 321 + calls, nil
	}
	command, active := enforcer.Counters(cfg, options.HostOptions{})
	if !active || calls != 1 || flagValue(command.Args, "--network") != "ns:/proc/322/ns/net" {
		test.Fatalf("process snapshot not fixed in command: %+v", command)
	}
}

func TestMissingManifestCounterIsErrorNotIsolation(t *testing.T) {
	cfg, _ := configured()
	enforcer := Enforcer{Load: func(schema.AppConfig) (provision.Manifest, error) {
		return provision.Manifest{}, errors.New("manifest missing")
	}}
	command, active := enforcer.Counters(cfg, options.HostOptions{})
	if !active || !strings.Contains(strings.Join(command.Args, " "), "exit 1") {
		t.Fatal("failed inspection claimed isolation")
	}
}

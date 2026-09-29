package netenforce

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func configured() (schema.AppConfig, provision.Manifest) {
	cfg := schema.AppConfig{Type: schema.ZincContainer, AppNameID: "client", NetworkMeta: schema.NetworkMeta{
		Interfaces: []schema.NetworkInterface{{ID: "main"}}, RulesByPriority: []schema.NetworkRule{{
			From: schema.NetworkPeer{Type: schema.NetworkPeerSelf}, To: schema.NetworkPeer{Type: schema.NetworkPeerAny},
			Protocols: []schema.NetworkProtocol{schema.NetworkTCP},
		}},
	}}
	manifest := provision.Manifest{Version: 1, AppNameID: "client", Generation: "launch-1", NetworkNamespace: "/run/zinc/client.net", UserNamespace: "/run/zinc/client.user",
		NetworkInode: 123, UserInode: 456, PacketPreserving: true, Exclusive: true, StaticNeighbors: true, CompleteInventory: true, Policy: cfg.NetworkMeta,
		Topology: network.Topology{Mode: network.Container, Interfaces: []network.Attachment{{InterfaceID: "main", Device: "eth0", MAC: "02:00:00:00:00:01", Addresses: []string{"10.0.0.2"}}}},
	}
	return cfg, manifest
}

func adapter(manifest provision.Manifest) Enforcer {
	return Enforcer{
		Load:          func(schema.AppConfig) (provision.Manifest, error) { return manifest, nil },
		UserNamespace: func() (uint64, error) { return 789, nil },
		ProcessID:     func(string) (int, error) { return 321, nil },
	}
}

func TestPrepareLocksBeforePodAndPreservesPrivileges(t *testing.T) {
	cfg, manifest := configured()
	enforcer := adapter(manifest)
	steps, err := enforcer.Prepare(cfg, options.HostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Args[0] != "run" || steps[1].Args[0] != "pod" {
		t.Fatalf("bad sequence: %+v", steps)
	}
	for _, wanted := range []string{"--cap-drop", "all", "--cap-add", "NET_ADMIN", "--user", "0", "no-new-privileges", "never", "ns:" + manifest.NetworkNamespace, "ns:" + manifest.UserNamespace} {
		if !slices.Contains(steps[0].Args, wanted) {
			t.Errorf("helper missing %s", wanted)
		}
	}
	if !strings.Contains(steps[0].Stdin, "policy drop;") || !strings.Contains(steps[0].Stdin, "rule[0]") {
		t.Fatal("policy not passed on stdin")
	}
	if !reflect.DeepEqual(enforcer.RunFlags(cfg), []string{"--pod", "client-pod"}) {
		t.Fatal("app does not join locked pod")
	}
	joined := strings.Join(steps[1].Args, " ")
	if !strings.Contains(joined, "--dns none") {
		t.Fatal("undeclared DNS inherited the host resolver")
	}
	if strings.Contains(joined, "pasta") || strings.Contains(joined, " -p ") || strings.Contains(joined, "NET_ADMIN") {
		t.Fatal("implicit uplink/publication or app network privileges")
	}
}

func TestNoNICAndMissingProvisioning(t *testing.T) {
	cfg, _ := configured()
	cfg.NetworkMeta.Interfaces = nil
	enforcer := Enforcer{Load: func(schema.AppConfig) (provision.Manifest, error) {
		t.Fatal("isolated app loaded topology")
		return provision.Manifest{}, nil
	}}
	steps, err := enforcer.Prepare(cfg, options.HostOptions{})
	if err != nil || len(steps) != 0 || !reflect.DeepEqual(enforcer.RunFlags(cfg), []string{"--network", "none"}) {
		t.Fatal("empty interfaces opened network")
	}
	cfg, _ = configured()
	enforcer.Load = func(schema.AppConfig) (provision.Manifest, error) {
		return provision.Manifest{}, errors.New("missing topology")
	}
	if steps, err = enforcer.Prepare(cfg, options.HostOptions{}); err == nil || len(steps) != 0 {
		t.Fatal("missing topology produced partial launch")
	}
}

func TestKeepIDAndHelperOverride(t *testing.T) {
	cfg, manifest := configured()
	cfg.InternalUserMeta.KeepUserID = true
	if _, err := adapter(manifest).Prepare(cfg, options.HostOptions{}); err == nil {
		t.Fatal("ignored keep-id mismatch")
	}
	manifest.KeepUserID = true
	steps, err := adapter(manifest).Prepare(cfg, options.HostOptions{NetfilterImage: "localhost/custom:local"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(steps[0].Args, "localhost/custom:local") {
		t.Fatal("helper override ignored")
	}
}

func TestTeardownClosesButDoesNotDestroyProvisionerTopology(t *testing.T) {
	cfg, manifest := configured()
	steps := adapter(manifest).Teardown(cfg)
	if len(steps) != 2 || steps[0].Args[0] != "pod" || !slices.Contains(steps[0].Args, "--ignore") {
		t.Fatal("pod teardown not idempotent")
	}
	joined := strings.Join(steps[1].Args, " ")
	if !strings.Contains(joined, "policy drop;") || strings.Contains(joined, "network rm") || strings.Contains(joined, "ip link del") || strings.Contains(joined, "wg ") {
		t.Fatal("teardown lost closure or destroys provisioned topology")
	}
	cfg.NetworkMeta.Interfaces = nil
	if steps = adapter(manifest).Teardown(cfg); len(steps) != 1 || steps[0].Args[0] != "rm" {
		t.Fatal("isolated teardown changed")
	}
}

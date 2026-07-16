package validate

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// A newline in an Install step would break out of the derived-image RUN line and let a
// crafted config inject its own Containerfile directives (a second FROM that swaps the
// base to an unpinned image), defeating the digest pin. It must be rejected.
func TestInstallControlCharRejected(t *testing.T) {
	cfg := baseCfg()
	cfg.ImageMeta.Install = []string{"true\nFROM docker.io/attacker/x:latest\nRUN :"}
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("Install with a newline: want a control-characters error, got: %v", err)
	}
}

// A clean multi-step install (spaces allowed, one step per entry) passes.
func TestInstallCleanOK(t *testing.T) {
	cfg := baseCfg()
	cfg.ImageMeta.Install = []string{"apk add --no-cache firefox", "adduser -D app"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("clean multi-step Install: want nil, got: %v", err)
	}
}

// A DependsOn name is joined into a store path, so a "../.." value could read an app
// definition from outside the apps directory. It must be charset-checked like AppNameID.
func TestDependsOnTraversalRejected(t *testing.T) {
	cfg := baseCfg()
	cfg.StartConditions.DependsOn = []string{"../../../../etc/evil"}
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "DependsOn") {
		t.Fatalf("DependsOn with a traversal path: want a DependsOn error, got: %v", err)
	}
}

func TestDependsOnValidOK(t *testing.T) {
	cfg := baseCfg()
	cfg.StartConditions.DependsOn = []string{"vpn", "db-1"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("valid DependsOn names: want nil, got: %v", err)
	}
}

// A filtered app (one with NetworkLists) runs in the pod netns that carries the nft
// egress lock-down; granting NET_ADMIN (or the superset SYS_ADMIN) would let it flush
// the ruleset and escape the filter. Both bare and CAP_-prefixed forms are rejected.
func TestNetworkCapabilityOnFilteredAppRejected(t *testing.T) {
	for _, capability := range []string{"NET_ADMIN", "CAP_SYS_ADMIN"} {
		cfg := withList(schema.NetworkList{Ingress: true, Ports: []int{5432}})
		cfg.Capabilities = []string{capability}
		err := Validate(cfg)
		if err == nil || !strings.Contains(err.Error(), "escape the network filter") {
			t.Fatalf("%s on a filtered app: want an egress-escape error, got: %v", capability, err)
		}
	}
}

// An isolated app (no NetworkLists) runs with --network none, so NET_ADMIN reaches
// nothing; it is not rejected.
func TestNetworkCapabilityOnIsolatedAppOK(t *testing.T) {
	cfg := baseCfg()
	cfg.Capabilities = []string{"NET_ADMIN"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("NET_ADMIN on an isolated app: want nil, got: %v", err)
	}
}

// A benign capability on a filtered app is fine.
func TestBenignCapabilityOnFilteredAppOK(t *testing.T) {
	cfg := withList(schema.NetworkList{Ingress: true, Ports: []int{5432}})
	cfg.Capabilities = []string{"NET_BIND_SERVICE"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("benign cap on a filtered app: want nil, got: %v", err)
	}
}

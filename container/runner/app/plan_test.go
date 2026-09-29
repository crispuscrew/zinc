package app

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/adapters/dbusproxy"
	"github.com/crispuscrew/zinc/container/runner/adapters/netenforce"
	"github.com/crispuscrew/zinc/container/runner/adapters/podman"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

const digestPin = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func baseOpts() options.HostOptions {
	return options.HostOptions{RuntimeDir: "/run/user/1000", WaylandDisplay: "wayland-1", HomeDir: "/root"}
}
func planSvc() Service {
	return New(nil, podman.Runtime{}, nil, nil, netenforce.Enforcer{}, dbusproxy.Broker{}, nil, nil, nil)
}
func assertContainsSeq(t *testing.T, args []string, first, second string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return
		}
	}
	t.Fatalf("missing adjacent %q %q in %v", first, second, args)
}
func mustNotContain(t *testing.T, args []string, unwanted string) {
	t.Helper()
	if slices.Contains(args, unwanted) {
		t.Fatalf("unexpected %q in %v", unwanted, args)
	}
}

type preparedNetwork struct{ failure error }

func (network preparedNetwork) RunFlags(schema.AppConfig) []string {
	return []string{"--pod", "demo-pod"}
}
func (network preparedNetwork) Prepare(schema.AppConfig, options.HostOptions) ([]ports.Command, error) {
	return []ports.Command{{Args: []string{"run", "helper", "nft"}, Desc: "lock network"}, {Args: []string{"pod", "create"}, Desc: "attach pod"}}, network.failure
}
func (network preparedNetwork) Teardown(schema.AppConfig) []ports.Command { return nil }
func (network preparedNetwork) Counters(schema.AppConfig, options.HostOptions) (ports.Command, bool) {
	return ports.Command{}, false
}

func TestPlanIsolated(t *testing.T) {
	plan, err := planSvc().Plan(depApp("tool"), baseOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 {
		t.Fatal(plan)
	}
	assertContainsSeq(t, plan[0].Args, "--network", "none")
}

func TestPlanKeepsRawFlagsOffHelpers(t *testing.T) {
	cfg := depApp("demo")
	cfg.RunnerFlags = []string{"--privileged", "--label=raw"}
	cfg.MinimizeFingerprint = true
	cfg.StartConditions = schema.StartConditions{Terminal: true, Attached: true, Entrypoint: "htop"}
	svc := New(nil, podman.Runtime{}, nil, nil, preparedNetwork{}, dbusproxy.Broker{}, nil, nil, nil)
	plan, err := svc.Plan(cfg, baseOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 3 || plan[0].Desc != "lock network" {
		t.Fatal(plan)
	}
	for _, helper := range plan[:2] {
		mustNotContain(t, helper.Args, "--privileged")
		mustNotContain(t, helper.Args, "--label=raw")
	}
	assertContainsSeq(t, plan[1].Args, "--hostname", "localhost")
	mustNotContain(t, plan[0].Args, "--hostname")
	mustNotContain(t, plan[2].Args, "--hostname")
	assertContainsSeq(t, plan[2].Args, "--cap-drop", "all")
	if !slices.Contains(plan[2].Args, "--unsetenv=container") {
		t.Fatal("missing marker minimization")
	}
	assertContainsSeq(t, plan[2].Args, "--pod", "demo-pod")
	assertContainsSeq(t, plan[2].Args, "--cap-drop", "all")
	assertContainsSeq(t, plan[2].Args, "--privileged", "--label=raw")
	if !slices.Equal(plan[2].Args[len(plan[2].Args)-2:], podman.HolderCmd()) {
		t.Fatal(plan)
	}
}

func TestLaunchFailsBeforePortsOnInvalidConfig(t *testing.T) {
	cfg := depApp("demo")
	cfg.ImageMeta.Image = "alpine:latest"
	if err := (Service{}).Launch(cfg, options.HostOptions{}); err == nil || !strings.Contains(err.Error(), "digest-pinned") {
		t.Fatal(err)
	}
}

func TestNetworkPreparationFailurePrecedesDependencies(t *testing.T) {
	engine := newFakeRuntime()
	svc := New(nil, engine, nil, nil, preparedNetwork{errors.New("namespace unavailable")}, dbusproxy.Broker{}, nil, nil, nil)
	if err := svc.Launch(depApp("demo", "missing"), baseOpts()); err == nil || !strings.Contains(err.Error(), "namespace unavailable") {
		t.Fatal(err)
	}
	if len(engine.commands) > 0 || len(engine.started) > 0 {
		t.Fatal("network failure reached runtime")
	}
}

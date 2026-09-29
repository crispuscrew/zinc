package app

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func TestSessionEnvironmentPropagatesThroughPort(t *testing.T) {
	cfg := depApp("demo")
	cfg.StartConditions = schema.StartConditions{Terminal: true, Attached: true, Entrypoint: "fallback", AttachedEntrypoint: `printf '%s' "two words"`,
		EntrypointEnv: map[string]string{"BASE": "keep", "VALUE": "old"}, AttachedEnv: map[string]string{"VALUE": "new", "EMPTY": ""}}
	engine := newFakeRuntime("demo")
	svc := depSvc(nil, engine)
	if err := svc.runTerminalSession(cfg, baseOpts(), false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(engine.sessionCmd, []string{"/bin/sh", "-c", cfg.StartConditions.AttachedEntrypoint}) {
		t.Fatal(engine.sessionCmd)
	}
	if !reflect.DeepEqual(engine.sessionEnv, map[string]string{"BASE": "keep", "VALUE": "new", "EMPTY": ""}) {
		t.Fatal(engine.sessionEnv)
	}
	engine.sessionEnv["BASE"] = "changed"
	if cfg.StartConditions.EntrypointEnv["BASE"] != "keep" {
		t.Fatal("session mutated definition")
	}
	if err := svc.runTerminalSession(cfg, baseOpts(), true); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(engine.sessionCmd, []string{"/bin/sh"}) || engine.sessionEnv["VALUE"] != "new" {
		t.Fatal("shell lost session environment")
	}
}

func TestHolderReusesLiveAndReplacesExited(t *testing.T) {
	cfg := depApp("demo")
	engine := newFakeRuntime("demo")
	svc := depSvc(nil, engine)
	if err := svc.ensureHolder(cfg, baseOpts()); err != nil {
		t.Fatal(err)
	}
	if len(engine.commands) != 0 {
		t.Fatal("reopen touched live holder")
	}
	engine.running["demo"], engine.exited = false, true
	if err := svc.ensureHolder(cfg, baseOpts()); err != nil {
		t.Fatal(err)
	}
	if len(engine.commands) != 2 || engine.commands[0].Args[0] != "rm" || engine.commands[1].Args[0] != "run" {
		t.Fatal(engine.commands)
	}
}

func TestTerminalRequestPreservesResolvedLaunch(t *testing.T) {
	cfg := depApp("demo.work")
	cfg.StartConditions.AttachedEnv = map[string]string{"SESSION": "new"}
	cfg.Volumes = []schema.Volume{{HostMounted: true, HostMount: "/one-run", InnerMount: "/data"}}
	want := TerminalRequest{Config: cfg, Options: options.HostOptions{BundleDir: "/apps/demo/configs", Terminal: []string{"foot", "--title=work"}}}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got TerminalRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("request changed: %+v", got)
	}
}

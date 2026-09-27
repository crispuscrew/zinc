package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Exercise the real detached handoff while replacing only the terminal process.
func TestMain(tests *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__term" && os.Getenv("ZINC_TEST_TERMINAL_REQUEST") != "" {
		var request TerminalRequest
		if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
			reportTerm("error " + err.Error())
			os.Exit(1)
		}
		data, err := json.Marshal(request)
		if err == nil {
			err = os.WriteFile(os.Getenv("ZINC_TEST_TERMINAL_REQUEST"), data, 0o600)
		}
		if err != nil {
			reportTerm("error " + err.Error())
			os.Exit(1)
		}
		reportTerm("ok")
		os.Exit(0)
	}
	os.Exit(tests.Run())
}

func TestLaunchReopensAttachedWithFreshEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	t.Setenv("ZINC_TEST_TERMINAL_REQUEST", path)
	cfg := depApp("demo.work")
	cfg.StartConditions = schema.StartConditions{Terminal: true, Attached: true, Entrypoint: "printf '%s' \"$SESSION\""}
	cfg.CreatorFlags = []string{"--label=raw"} // A running holder must not rebuild.
	cfg.Configs = []schema.ConfigFile{{BundlePath: "gone.json", InnerMount: "/etc/app.json"}}
	engine := newFakeRuntime(cfg.AppNameID)
	svc := depSvc(nil, engine)
	opt := baseOpts()
	opt.Terminal = []string{"foot"}
	for launch := 0; launch < 2; launch++ {
		value := fmt.Sprint(launch)
		cfg.StartConditions.AttachedEnv = map[string]string{"SESSION": value}
		if err := svc.Launch(cfg, opt); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var request TerminalRequest
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		if request.Config.StartConditions.AttachedEnv["SESSION"] != value || request.Config.AppNameID != cfg.AppNameID {
			t.Fatalf("stale request: %+v", request)
		}
	}
	if len(engine.commands) != 0 || len(engine.started) != 0 {
		t.Fatal("reopen recreated live holder")
	}
}

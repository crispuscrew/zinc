package app

import (
	"os"
	"strings"
	"testing"
)

func TestExecutingLaunchEmitsRawFlagWarning(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "warnings")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	previous := os.Stderr
	os.Stderr = output
	defer func() { os.Stderr = previous }()
	cfg := depApp("demo")
	cfg.RunnerFlags = []string{"--privileged"}
	engine := newFakeRuntime()
	if err := depSvc(nil, engine).Launch(cfg, baseOpts()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "WARNING:") || !strings.Contains(string(data), "RunnerFlags") {
		t.Fatalf("execution hid raw flag warning: %s", data)
	}
	if len(engine.started) != 1 {
		t.Fatal("test did not execute launch")
	}
}

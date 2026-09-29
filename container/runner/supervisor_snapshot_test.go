package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func TestSupervisorIgnoresStoreEditsBeforeReadiness(t *testing.T) {
	root, source := supervisorFixture(t)
	t.Setenv(supervisorTestMode, "delayed")
	svc := supervisorService(root, false)
	want, err := loadLaunchable(svc, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "apps"), 0o700); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(root, "apps", "client.yaml")
	data, err := svc.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stored, data, 0o600); err != nil {
		t.Fatal(err)
	}
	want, err = loadLaunchable(svc, "client")
	if err != nil {
		t.Fatal(err)
	}
	launched := make(chan error, 1)
	go func() { launched <- cmdRun(svc, options.HostOptions{}, []string{"client", "--exec"}) }()
	awaitSupervisorFile(t, filepath.Join(root, "decoding"))
	select {
	case err := <-launched:
		t.Fatalf("launch returned before supervisor readiness: %v", err)
	default:
	}
	if err := os.WriteFile(stored, []byte("invalid: edited while helper starts\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "decode-now"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-launched:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("launch did not finish after readiness")
	}
	awaitSupervisorFile(t, filepath.Join(root, "waiting"))
	if err := os.WriteFile(filepath.Join(root, "exited"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var got schema.AppConfig
	if err := json.Unmarshal(awaitSupervisorFile(t, filepath.Join(root, "cleaned.json")), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("supervisor reloaded the store: %+v", got)
	}
}

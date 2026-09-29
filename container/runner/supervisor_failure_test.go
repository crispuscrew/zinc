package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/container/runner/adapters/ipc"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func TestSupervisorStartupFailureCleansFileLaunch(t *testing.T) {
	for _, test := range []struct{ mode, message string }{
		{"silent", "supervise client"}, {"identity", "identity mismatch"},
		{"observer", "runtime unavailable"}, {"app-failure", "app startup failed"},
		{"oversized", "helper message exceeds"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			root, path := supervisorFixture(t)
			t.Setenv(supervisorTestMode, test.mode)
			svc := supervisorService(root, false)
			if test.mode == "oversized" {
				cfg, err := loadLaunchable(svc, path)
				if err != nil {
					t.Fatal(err)
				}
				cfg.StartConditions.EntrypointEnv["SESSION"] = strings.Repeat("x", ipc.Maximum)
				data, err := svc.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err := cmdRun(svc, options.HostOptions{}, []string{path, "--exec"})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("startup error: %v", err)
			}
			awaitSupervisorFile(t, filepath.Join(root, "cleaned.json"))
			for _, name := range []string{"resource", "started", "waiting"} {
				if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
					t.Fatalf("failed launch retained %s", name)
				}
			}
		})
	}
}

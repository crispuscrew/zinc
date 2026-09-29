package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestListBuildDelegatesForInstallOrCreatorFlags(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		install      []string
		creatorFlags []string
		wantBuild    bool
	}{
		{"flags-only", nil, []string{"--build-arg=MODE=work"}, true},
		{"install-only", []string{"touch /ready"}, nil, true},
		{"both", []string{"touch /ready"}, []string{"--build-arg=MODE=work"}, true},
		{"no-build", nil, nil, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			model, storage := newLoaded(t, "shell")
			config := sample("shell")
			config.ImageMeta.Install, config.CreatorFlags = testCase.install, testCase.creatorFlags
			if err := storage.Save(config); err != nil {
				t.Fatal(err)
			}
			model = send(model, loadApps(model.svc)())
			directory := t.TempDir()
			calls := filepath.Join(directory, "calls")
			t.Setenv("BUILD_CALLS", calls)
			t.Setenv("PATH", directory)
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$BUILD_CALLS\"\nprintf 'build complete\\n'\n"
			if err := os.WriteFile(filepath.Join(directory, "zcr"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			updated, command := model.Update(key("b"))
			if !testCase.wantBuild {
				if command != nil || !strings.Contains(updated.(Model).status, "nothing to build") {
					t.Fatal("config without Install or CreatorFlags must not schedule a build")
				}
				if _, err := os.Stat(calls); !os.IsNotExist(err) {
					t.Fatalf("unexpected runtime call: %v", err)
				}
				return
			}
			if command == nil {
				t.Fatalf("build was blocked: %s", updated.(Model).status)
			}
			message := command()
			status, valid := message.(statusMsg)
			if !valid || !strings.Contains(status.text, "built image for shell") || !strings.Contains(status.text, "build complete") {
				t.Fatalf("build did not report successful delegation: %#v", message)
			}
			data, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "build\nshell\n" {
				t.Fatalf("runtime arguments = %q, want build and original store key", data)
			}
		})
	}
}

func TestListBuildStillRejectsVMWithCreatorFlags(t *testing.T) {
	model, _ := newLoaded(t, "guest")
	model.apps[0].cfg.Type = schema.ZincVirtualization
	model.apps[0].cfg.CreatorFlags = []string{"--raw-flag"}
	updated, command := model.Update(key("b"))
	if command != nil || !strings.Contains(updated.(Model).status, "nothing to build") {
		t.Fatal("CreatorFlags must not enable the container build action for a VM")
	}
}

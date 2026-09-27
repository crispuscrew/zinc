package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/store"
)

func TestActionsDispatchVMAndKeepSuccessfulWarnings(t *testing.T) {
	directory := t.TempDir()
	for _, binary := range []string{"zcr", "zvr"} {
		script := "#!/bin/sh\nprintf '%s %s %s' '" + binary + "' \"$1\" \"$2\"\nprintf 'backend warning' >&2\n"
		if err := os.WriteFile(filepath.Join(directory, binary), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	service := New(&store.Store{Root: t.TempDir()})
	cfg := validApp("guest")
	cfg.Type, cfg.ImageMeta.Image = schema.ZincVirtualization, "/images/base.qcow2"
	cfg.ResourcesMeta = schema.ResourcesMeta{MaxCPUCores: 2, MaxRamMiB: 4096}
	cfg.RunnerFlags = []string{"-name", "raw-name"}
	if err := service.Save(cfg); err != nil {
		t.Fatal(err)
	}
	report, err := service.Action("run", "guest", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"zvr run guest", "backend warning", "Raw backend flags"} {
		if !strings.Contains(report, expected) {
			t.Errorf("missing %q: %s", expected, report)
		}
	}
	if _, err := service.Action("build", "guest", false); err == nil {
		t.Fatal("VM build sent to container runtime")
	}
}

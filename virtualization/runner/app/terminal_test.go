package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestTerminalConfigurationIsExplicit(t *testing.T) {
	t.Setenv("ZINC_TERMINAL", "")
	t.Setenv("TERMINAL", "")
	cfg := schema.AppConfig{}
	if args, err := terminalCommand(cfg); err != nil || len(args) != 0 {
		t.Fatal(args, err)
	}
	cfg.StartConditions.Terminal = true
	if _, err := terminalCommand(cfg); err == nil {
		t.Fatal("missing terminal configuration accepted")
	}
	root := t.TempDir()
	for _, binary := range []string{"terminal", "socat"} {
		if err := os.WriteFile(filepath.Join(root, binary), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", root)
	t.Setenv("ZINC_TERMINAL", `["terminal","--title","two words"]`)
	args, err := terminalCommand(cfg)
	if err != nil || len(args) != 3 || args[2] != "two words" {
		t.Fatal(args, err)
	}
}

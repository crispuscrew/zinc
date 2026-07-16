// Command hzl is the HyprZinc Launcher — the keyboard-first launcher and smart
// executor (Super+G). The gioui search UI arrives in milestone M7 (see
// ../ROADMAP.md). Until then this exposes the launcher's core action on the CLI:
// `hzl <app>` resolves a defined app from the shared store and launches it through
// the SAME application service hzc uses (core/app via core/wire) — so the run logic
// lives in exactly one place and hzl reuses it rather than reimplementing
// (docs/architecture.md §9.1, §13).
package main

import (
	"fmt"
	"os"

	"github.com/crispuscrew/hyprzinc/core/adapters/host"
	"github.com/crispuscrew/hyprzinc/core/wire"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hzl: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) != 1 || argv[0] == "-h" || argv[0] == "--help" {
		return fmt.Errorf("usage: hzl <app>\n  launch a defined app via the shared service (gioui search UI: M7)")
	}
	svc, err := wire.DefaultService()
	if err != nil {
		return err
	}
	cfg, err := svc.Load(argv[0])
	if err != nil {
		return err
	}
	return svc.Launch(cfg, host.Options())
}

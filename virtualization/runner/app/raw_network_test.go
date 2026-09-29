package app

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

func TestNoNICIsolationHasOnlyExplicitRawEscape(check *testing.T) {
	svc, cfg := serviceFixture(check)
	svc.Paths.RunDir = "/run/zinc-test"
	plan, err := svc.launchPlan(cfg)
	if err != nil {
		check.Fatal(err)
	}
	forged := append(qemu.Args(cfg, plan.Layout), "-netdev", "user,id=raw")
	if _, _, err := svc.networkCommand(plan, forged); err == nil {
		check.Fatal("undeclared NIC accepted without raw opt-out")
	}
	cfg.RunnerFlags = []string{"-netdev", "user,id=raw"}
	plan, err = svc.launchPlan(cfg)
	if err != nil {
		check.Fatal(err)
	}
	args, _, err := svc.networkCommand(plan, qemu.Args(cfg, plan.Layout))
	if err != nil || !strings.Contains(strings.Join(args, " "), "user,id=raw") {
		check.Fatal(args, err)
	}
	warnings := strings.Join(qemu.Warnings(cfg, svc.Options, false), " ")
	if !strings.Contains(warnings, "not guaranteed network-isolated") {
		check.Fatal(warnings)
	}
}

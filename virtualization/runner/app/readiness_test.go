package app

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
)

func TestPreparationBudgetDefaultsAndValidation(check *testing.T) {
	budget, err := (Service{}).preparationBudget()
	if err != nil || budget != DefaultPreparationTimeout || budget <= 2*ipc.ReadyTimeout {
		check.Fatalf("default preparation budget = %s, %v", budget, err)
	}
	svc := Service{PreparationTimeout: -time.Second}
	if err := svc.Run(schema.AppConfig{}); err == nil || err.Error() != "VM preparation timeout must not be negative" {
		check.Fatalf("invalid budget did not fail before app preparation: %v", err)
	}
}

func TestPreparationCanExceedMessageDeadline(check *testing.T) {
	child, request := startPreparation(check, 60*time.Millisecond)
	// A prior short message deadline must not govern the subsequent preparation.
	if err := child.Status.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		check.Fatal(err)
	}
	svc := Service{PreparationTimeout: 2 * time.Second}
	if err := svc.awaitSupervisor(child); err != nil {
		check.Fatal(err)
	}
	assertStartupResult(check, child, request, "committed")
}

func TestPreparationTimeoutRollsBackUnacceptedStartup(check *testing.T) {
	child, request := startPreparation(check, 100*time.Millisecond)
	svc := Service{PreparationTimeout: 20 * time.Millisecond}
	err := svc.awaitSupervisor(child)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		check.Fatalf("want bounded preparation timeout, got %v", err)
	}
	assertStartupResult(check, child, request, "cleaned")
}

func TestDisconnectedParentRollsBackBeforeAcknowledgement(check *testing.T) {
	child, request := startPreparation(check, 20*time.Millisecond)
	// EOF after the request is not acceptance, even if preparation succeeds.
	if err := child.Control.Close(); err != nil {
		check.Fatal(err)
	}
	var ready LaunchReady
	if err := ipc.ReceiveWithin(child.Status, &ready, 2*time.Second); err != nil {
		check.Fatal(err)
	}
	if ready.PID <= 1 {
		check.Fatal("helper did not reach readiness")
	}
	assertStartupResult(check, child, request, "cleaned")
}

func TestInvalidAcknowledgementRollsBackStartup(check *testing.T) {
	child, request := startPreparation(check, 0)
	var ready LaunchReady
	if err := ipc.ReceiveWithin(child.Status, &ready, 2*time.Second); err != nil {
		check.Fatal(err)
	}
	if _, err := child.Control.Write([]byte{2}); err != nil {
		check.Fatal(err)
	}
	child.Control.Close()
	assertStartupResult(check, child, request, "cleaned")
}

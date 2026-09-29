package app

import (
	"testing"
	"time"
)

func TestStopWaitsForAnInProgressStartup(check *testing.T) {
	svc, cfg := serviceFixture(check)
	lock, err := svc.lock(cfg.AppNameID)
	if err != nil {
		check.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- svc.Stop(cfg.AppNameID, true, time.Second) }()
	select {
	case err := <-result:
		lock.Close()
		check.Fatalf("stop returned before startup released its lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	lock.Close()
	if err := <-result; err != nil {
		check.Fatal(err)
	}
}

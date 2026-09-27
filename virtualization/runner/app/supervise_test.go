package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/virtualization/runner/adapters/audioholder"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/machine"
)

func TestManualStopCancelsBackoffButNotFutureGeneration(check *testing.T) {
	svc, cfg := serviceFixture(check)
	if err := svc.Paths.EnsureDirs(); err != nil {
		check.Fatal(err)
	}
	generation := strings.Repeat("a", 32)
	identity, err := machine.ProcessIdentity(os.Getpid())
	if err != nil {
		check.Fatal(err)
	}
	body, err := json.Marshal(supervisorRecord{os.Getpid(), identity, generation})
	if err != nil {
		check.Fatal(err)
	}
	if err := svc.writeControl(cfg.AppNameID, "supervisor.json", body); err != nil {
		check.Fatal(err)
	}
	if err := svc.Stop(cfg.AppNameID, false, time.Millisecond); err != nil {
		check.Fatal(err)
	}
	started := time.Now()
	if waitRestart(context.Background(), time.Minute, func() bool { return svc.stopRequested(cfg.AppNameID, generation) }) {
		check.Fatal("manual stop allowed restart")
	}
	if time.Since(started) > time.Second {
		check.Fatal("manual stop waited out backoff")
	}
	if svc.stopRequested(cfg.AppNameID, strings.Repeat("b", 32)) {
		check.Fatal("old stop canceled a new launch")
	}
}

func TestRestartBackoffIsBoundedAndCancelable(check *testing.T) {
	var previous time.Duration
	for attempt := 0; attempt < 5; attempt++ {
		delay, allowed := restartDelay(attempt)
		if !allowed || delay <= previous || delay > 16*time.Second {
			check.Fatal(attempt, delay, allowed)
		}
		previous = delay
	}
	if _, allowed := restartDelay(5); allowed {
		check.Fatal("unbounded restart loop")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitRestart(ctx, time.Minute, func() bool { return false }) {
		check.Fatal("cancellation ignored")
	}
}

func TestAudioFailureReportsRevocationWithoutStoppingGuest(check *testing.T) {
	svc, cfg := serviceFixture(check)
	if err := svc.Paths.EnsureDirs(); err != nil {
		check.Fatal(err)
	}
	guestExit := make(chan error, 1)
	audioExit := make(chan struct{})
	handle := &audioholder.Handle{Child: &ipc.Child{Done: audioExit}}
	guest := execution{Process: &machine.Process{PID: 12345, Done: guestExit}, Audio: handle}
	finished := make(chan error, 1)
	go func() { finished <- svc.waitExecution(context.Background(), cfg.AppNameID, guest, io.Discard) }()
	close(audioExit)
	deadline := time.Now().Add(2 * time.Second)
	for {
		body, err := os.ReadFile(svc.controlPath(cfg.AppNameID, "fault"))
		if err == nil && strings.Contains(string(body), "guest retained") {
			break
		}
		if time.Now().After(deadline) {
			check.Fatal("audio revocation was not reported")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-finished:
		check.Fatal("audio failure ended guest supervision")
	default:
	}
	guestExit <- nil
	if err := <-finished; err != nil {
		check.Fatal(err)
	}
}

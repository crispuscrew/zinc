package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// Backoff is bounded per failure burst. Five minutes of stable execution resets
// the burst; a clean QEMU exit, manual stop or preparation failure never restarts.
func restartDelay(attempt int) (time.Duration, bool) {
	if attempt < 0 || attempt >= 5 {
		return 0, false
	}
	return time.Second * time.Duration(1<<attempt), true
}

func (svc Service) supervise(ctx context.Context, cfg schema.AppConfig, generation string,
	confirm func(int) error, diagnostic io.Writer) error {
	confirmed, attempt := false, 0
	for {
		if ctx.Err() != nil || svc.stopRequested(cfg.AppNameID, generation) {
			return nil
		}
		started := time.Now()
		guest, err := svc.startOnce(ctx, cfg, generation, diagnostic)
		if err != nil {
			if confirmed && (ctx.Err() != nil || svc.stopRequested(cfg.AppNameID, generation)) {
				return nil
			}
			svc.reportFault(cfg.AppNameID, err, diagnostic)
			return err
		}
		if !confirmed {
			stop := func(pid int) error { return svc.stop(cfg.AppNameID, false, DefaultStopTimeout, pid) }
			if err := acceptStartup(guest, confirm, stop); err != nil {
				return err
			}
			confirmed = true
		}
		exit := svc.waitExecution(ctx, cfg.AppNameID, guest, diagnostic)
		cleanup := errors.Join(guest.Cleanup(), svc.Runtime.Cleanup(cfg.AppNameID))
		manual := ctx.Err() != nil || svc.stopRequested(cfg.AppNameID, generation)
		if manual {
			return cleanup
		}
		if exit == nil || !cfg.StopConditions.Autorestart || cleanup != nil {
			result := combineExit(exit, cleanup)
			if result != nil {
				svc.reportFault(cfg.AppNameID, result, diagnostic)
			}
			return result
		}
		if time.Since(started) >= 5*time.Minute {
			attempt = 0
		}
		delay, allowed := restartDelay(attempt)
		if !allowed {
			err := fmt.Errorf("autorestart exhausted five retries: %w", exit)
			svc.reportFault(cfg.AppNameID, err, diagnostic)
			return err
		}
		attempt++
		svc.reportFault(cfg.AppNameID, fmt.Errorf("guest failed; restart %d/5 in %s: %w", attempt, delay, exit), diagnostic)
		if !waitRestart(ctx, delay, func() bool { return svc.stopRequested(cfg.AppNameID, generation) }) {
			return nil
		}
	}
}

func (svc Service) waitExecution(ctx context.Context, name string, guest execution, diagnostic io.Writer) error {
	var audioDone <-chan struct{}
	if guest.Audio != nil {
		audioDone = guest.Audio.Done
	}
	canceled := ctx.Done()
	for {
		select {
		case exit := <-guest.Process.Done:
			return exit
		case <-audioDone:
			failure := guest.Audio.Err()
			if failure == nil {
				failure = fmt.Errorf("audio holder exited while guest was running")
			}
			svc.reportFault(name, fmt.Errorf("audio revoked; guest retained: %w", failure), diagnostic)
			audioDone = nil // revocation is terminal for audio, not for guest storage
		case <-canceled:
			if err := svc.stop(name, false, DefaultStopTimeout, guest.Process.PID); err != nil {
				svc.reportFault(name, err, diagnostic)
			}
			canceled = nil
		}
	}
}

func waitRestart(ctx context.Context, delay time.Duration, stopped func() bool) bool {
	timer, ticker := time.NewTimer(delay), time.NewTicker(50*time.Millisecond)
	defer timer.Stop()
	defer ticker.Stop()
	for {
		if stopped() {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return !stopped()
		case <-ticker.C:
		}
	}
}

func (svc Service) reportFault(name string, failure error, diagnostic io.Writer) {
	fmt.Fprintln(diagnostic, "zvr:", failure)
	if err := svc.writeControl(name, "fault", []byte(failure.Error())); err != nil {
		fmt.Fprintln(diagnostic, "record fault:", err)
	}
}

package podman

import (
	"fmt"
	"os/exec"
	"time"
)

const (
	appearWindow  = 60 * time.Second
	appearPoll    = 250 * time.Millisecond
	restartWindow = 3 * time.Second
)

// WaitGone allows an app to appear after its brokers, and outlasts automatic restarts.
func WaitGone(name string) error {
	engine := Runtime{}
	deadline := time.Now().Add(appearWindow)
	for !engine.Exists(name) {
		if time.Now().After(deadline) {
			return fmt.Errorf("container %s did not appear within %s", name, appearWindow)
		}
		time.Sleep(appearPoll)
	}
	for {
		if err := exec.Command("podman", "wait", name).Run(); err != nil {
			return err
		}
		if !engine.Exists(name) || !engine.restartsWithin(name, restartWindow) {
			return nil
		}
	}
}

func (engine Runtime) restartsWithin(name string, window time.Duration) bool {
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		time.Sleep(appearPoll)
		running, err := isRunning(name)
		if err != nil {
			continue
		} // Unknown is not evidence that a live context may be revoked.
		if running {
			return true
		}
		if !engine.Exists(name) {
			return false
		}
	}
	return false
}

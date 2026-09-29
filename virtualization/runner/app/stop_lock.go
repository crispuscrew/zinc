package app

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Stop intent is already persisted before waiting. A bootstrap that holds the
// lifecycle lock must observe it before launching QEMU or scheduling a retry.
func (svc Service) stopLock(name string, timeout time.Duration) (*os.File, error) {
	if timeout <= 0 {
		timeout = DefaultStopTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		lock, err := svc.lock(name)
		if err == nil {
			return lock, nil
		}
		if !strings.Contains(err.Error(), "operation is in progress") {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("stop intent recorded; timed out waiting for %s startup to finish", name)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

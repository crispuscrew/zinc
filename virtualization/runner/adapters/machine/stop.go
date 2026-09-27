package machine

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/crispuscrew/zinc/virtualization/runner/adapters/qmp"
)

func (runtime Runtime) Stop(name string, force bool, timeout time.Duration) error {
	state, err := runtime.State(name)
	if err != nil {
		return err
	}
	if !state.Alive {
		return runtime.clean(name)
	}
	pid := state.PID
	if !force {
		session, err := qmp.Dial(runtime.Paths.QMP(name))
		if err == nil {
			var requestErr error
			if state.Guest == "shutdown" {
				_, requestErr = session.Execute("quit")
			} else {
				requestErr = session.Powerdown()
			}
			session.Close()
			if requestErr == nil && waitGone(pid, timeout) {
				return runtime.clean(name)
			}
		}
		fmt.Fprintf(os.Stderr, "zvr: %s did not shut down within %s, terminating the guest process\n", name, timeout)
	}
	if !isGuestProcess(pid, name) {
		return fmt.Errorf("guest process identity changed while stopping %s", name)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !isGone(err) {
		return err
	}
	if !waitGone(pid, termGrace) {
		if !isGuestProcess(pid, name) {
			return fmt.Errorf("guest process identity changed before forced stop")
		}
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !isGone(err) {
			return err
		}
		if !waitGone(pid, termGrace) {
			return fmt.Errorf("guest %s still exists after SIGKILL; state retained", name)
		}
	}
	return runtime.clean(name)
}

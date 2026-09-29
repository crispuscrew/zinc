package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/machine"
)

const startupTestCommand = "__test-supervisor-ready"

type startupTestRequest struct {
	PhaseDelay time.Duration
	Result     string
	Stopped    string
}

func TestMain(check *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == startupTestCommand {
		if err := startupTestHelper(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(check.Run())
}

// Exercise the real handshake and rollback with a prepared resource standing in
// for guest/helper state. No QEMU, network or host audio service is needed.
func startupTestHelper() error {
	status, input, lifetime, err := ipc.Inherited()
	if err != nil {
		return err
	}
	defer status.Close()
	defer input.Close()
	defer lifetime.Close()
	var request startupTestRequest
	if err := ipc.Receive(input, &request); err != nil {
		return err
	}
	if err := os.WriteFile(request.Result, []byte("prepared"), 0o600); err != nil {
		return err
	}
	for range 2 {
		time.Sleep(request.PhaseDelay)
	}
	guest := execution{
		Process: &machine.Process{PID: os.Getpid()},
		Cleanup: func() error { return os.WriteFile(request.Result, []byte("cleaned"), 0o600) },
	}
	confirm := func(pid int) error { return confirmStartup(status, lifetime, pid) }
	stop := func(pid int) error {
		if pid != os.Getpid() {
			return fmt.Errorf("rollback targeted another guest")
		}
		return os.WriteFile(request.Stopped, []byte("stopped"), 0o600)
	}
	if err := acceptStartup(guest, confirm, stop); err != nil {
		return err
	}
	return os.WriteFile(request.Result, []byte("committed"), 0o600)
}

func startPreparation(check *testing.T, phase time.Duration) (*ipc.Child, startupTestRequest) {
	check.Helper()
	root := check.TempDir()
	request := startupTestRequest{phase, filepath.Join(root, "result"), filepath.Join(root, "stopped")}
	child, err := ipc.Start([]string{startupTestCommand}, request, io.Discard)
	if err != nil {
		check.Fatal(err)
	}
	child.Grace = time.Second
	check.Cleanup(func() { _ = child.Close() })
	return child, request
}

func assertStartupResult(check *testing.T, child *ipc.Child, request startupTestRequest, want string) {
	check.Helper()
	select {
	case <-child.Done:
	case <-time.After(2 * time.Second):
		check.Fatal("startup helper did not exit")
	}
	body, err := os.ReadFile(request.Result)
	if err != nil || string(body) != want {
		check.Fatalf("resource state = %q, %v; want %s", body, err, want)
	}
	_, err = os.Stat(request.Stopped)
	if want == "cleaned" && err != nil {
		check.Fatal("rollback omitted guest stop:", err)
	}
	if want == "committed" && !os.IsNotExist(err) {
		check.Fatal("accepted startup was rolled back")
	}
}

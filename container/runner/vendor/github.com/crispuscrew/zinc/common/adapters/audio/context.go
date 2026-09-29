package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (session *Session) createContext(request Request, instance string, deadline time.Time) error {
	var err error
	session.directory, err = os.MkdirTemp(request.RuntimeDir, "za-")
	if err != nil {
		return err
	}
	session.Socket = filepath.Join(session.directory, "pipewire-0")
	if len(session.Socket) >= 108 {
		return fmt.Errorf("audio: Unix socket path exceeds 107 bytes")
	}
	// Never unlink an existing instance's socket to satisfy Listen.
	handle, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return err
	}
	session.listener = os.NewFile(uintptr(handle), session.Socket)
	if err := syscall.Bind(handle, &syscall.SockaddrUnix{Name: session.Socket}); err != nil {
		return err
	}
	if err := os.Chmod(session.Socket, 0o600); err != nil {
		return err
	}
	if err := syscall.Listen(handle, 32); err != nil {
		return err
	}
	session.revokeRead, session.revokeWrite, err = os.Pipe()
	if err != nil {
		return err
	}
	session.context, err = connect(filepath.Join(request.RuntimeDir, "pipewire-0-manager"))
	if err != nil {
		return err
	}
	if err := session.context.hello("application.name", "Zinc audio context"); err != nil {
		return err
	}
	proxy, err := session.context.securityContext(deadline)
	if err != nil {
		return err
	}
	// ACCESS is immutable to clients. module-access leaves unknown categories
	// suspended; stock WirePlumber 0.5.14 does not grant this category anything.
	body := structure(descriptor(0), descriptor(1), dictionary(
		"pipewire.sec.engine", Engine, "pipewire.sec.app-id", request.AppID,
		"pipewire.sec.instance-id", instance, "pipewire.access", Access))
	if err := session.context.send(proxy, 1, body, handle, int(session.revokeRead.Fd())); err != nil {
		return err
	}
	if err := session.context.sync(deadline); err != nil {
		return err
	}
	// The security-context API keeps accepting after its creator disconnects.
	// Only the revocation pipe must remain open; do not leave an unread connection.
	err = session.context.socket.Close()
	session.context = nil
	return err
}

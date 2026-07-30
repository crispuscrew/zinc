package waylandctx

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
)

// fakeCompositor is just enough of a Wayland server to answer the exchange create() performs:
// it advertises the globals it is told to, answers syncs, and records what it was sent -
// including the descriptors, which are the part no encoding test can reach.
//
// It exists because the encoding tests prove the bytes and prove nothing about the ORDER, and
// the order is where this can fail invisibly: a bind before the registry exists, a setter
// after the commit, a create_listener whose fds went with the wrong message. A real compositor
// answers all of those with a protocol error and a dropped connection, and this one lets that
// be a test rather than something only reproducible on a desktop.
type fakeCompositor struct {
	path    string
	globals []string

	// failOnCommit makes the compositor answer the commit roundtrip with wl_display.error
	// instead of the callback, which is how a real one reports rejected metadata. Set before
	// the server is started, never after, so the serving goroutine only ever reads it.
	failOnCommit bool

	mutex     sync.Mutex
	bound     string   // the interface named in wl_registry.bind
	setters   []string // the strings the security context was given, in order
	setCodes  []uint16 // and the opcodes that carried them, so order is checked too
	fds       []int    // descriptors received with create_listener
	committed bool

	done chan struct{}
}

func startFakeCompositor(t *testing.T, failOnCommit bool, globals ...string) *fakeCompositor {
	t.Helper()
	srv := &fakeCompositor{
		path:         filepath.Join(t.TempDir(), "wayland-fake"),
		globals:      globals,
		failOnCommit: failOnCommit,
		done:         make(chan struct{}),
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: srv.path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		srv.serve(conn)
	}()
	return srv
}

// serve reads requests off the connection, keeping ancillary data, and answers the two syncs.
func (srv *fakeCompositor) serve(conn *net.UnixConn) {
	var (
		buf      []byte
		chunk    = make([]byte, 4096)
		oob      = make([]byte, syscall.CmsgSpace(4*4))
		registry uint32
		context  uint32
		syncs    int
	)
	for {
		read, oobRead, _, _, err := conn.ReadMsgUnix(chunk, oob)
		if read > 0 {
			buf = append(buf, chunk[:read]...)
		}
		if oobRead > 0 {
			srv.takeFDs(oob[:oobRead])
		}
		for {
			msg, size, perr := nextMessage(buf)
			if perr != nil || size == 0 {
				break
			}
			buf = buf[size:]
			switch {
			case msg.object == displayObject && msg.opcode == displayGetRegistry:
				registry = wire.Uint32(msg.body[0:4])
				for index, iface := range srv.globals {
					body := appendUint32(nil, uint32(index+1))
					body = appendString(body, iface)
					body = appendUint32(body, 1)
					conn.Write(request(registry, registryGlobalEvent, body))
				}
			case msg.object == displayObject && msg.opcode == displaySync:
				syncs++
				callback := wire.Uint32(msg.body[0:4])
				if syncs == 2 && srv.failOnCommit {
					body := appendUint32(nil, context)
					body = appendUint32(body, 3) // invalid_metadata
					body = appendString(body, "instance id is not unique")
					conn.Write(request(displayObject, displayErrorEvent, body))
					close(srv.done)
					return
				}
				conn.Write(request(callback, callbackDoneEvent, appendUint32(nil, 0)))
				if syncs == 2 {
					close(srv.done)
					return
				}
			case msg.object == registry && msg.opcode == registryBind:
				iface, _, _ := takeString(msg.body, 4)
				srv.record(func() { srv.bound = iface })
			case msg.opcode == managerCreateListener && msg.object != registry && context == 0:
				context = wire.Uint32(msg.body[0:4])
			case msg.object == context && msg.opcode == contextCommit:
				srv.record(func() { srv.committed = true })
			case msg.object == context:
				value, _, _ := takeString(msg.body, 0)
				srv.record(func() {
					srv.setters = append(srv.setters, value)
					srv.setCodes = append(srv.setCodes, msg.opcode)
				})
			}
		}
		if err != nil {
			return
		}
	}
}

func (srv *fakeCompositor) record(change func()) {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	change()
}

func (srv *fakeCompositor) takeFDs(oob []byte) {
	messages, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}
	for _, control := range messages {
		fds, err := syscall.ParseUnixRights(&control)
		if err != nil {
			continue
		}
		srv.record(func() { srv.fds = append(srv.fds, fds...) })
	}
}

func (srv *fakeCompositor) wait(t *testing.T) {
	t.Helper()
	select {
	case <-srv.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the fake compositor never finished the exchange")
	}
}

// The whole path, end to end against a server that speaks the protocol: bind the app's own
// socket, hand it over with a revocation pipe, attach the identity, commit. Then the two
// things the design actually rests on - that the descriptor handed over is a working
// listening socket for that path, and that closing the write end is what revokes it.
func TestCreateRegistersAndRevokes(t *testing.T) {
	srv := startFakeCompositor(t, false, "wl_compositor", managerInterface, "wl_seat")
	runtimeDir := t.TempDir()
	addr := paths.Address{App: "browser", Instance: "work"}
	opt := options.HostOptions{RuntimeDir: runtimeDir, WaylandDisplay: srv.path} // absolute display

	lis, err := create(addr, opt)
	if err != nil {
		t.Fatal(err)
	}
	srv.wait(t)

	want := SocketPath(runtimeDir, srv.path, addr)
	if lis.socket != want {
		t.Fatalf("socket path: got %q want %q", lis.socket, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("the app's socket must exist before the container is created: %v", err)
	}

	srv.mutex.Lock()
	bound, setters, codes, committed, fds := srv.bound, srv.setters, srv.setCodes, srv.committed, srv.fds
	srv.mutex.Unlock()

	if bound != managerInterface {
		t.Fatalf("bound %q, want %q", bound, managerInterface)
	}
	if !committed {
		t.Fatal("the context was never committed, so nothing was registered")
	}
	wantSetters := []string{SandboxEngine, "browser", "browser.work"}
	wantCodes := []uint16{contextSetSandboxEngine, contextSetAppID, contextSetInstanceID}
	if len(setters) != 3 {
		t.Fatalf("got %d setters, want 3: %v", len(setters), setters)
	}
	for index := range wantSetters {
		if setters[index] != wantSetters[index] || codes[index] != wantCodes[index] {
			t.Fatalf("setter %d: got opcode %d %q, want opcode %d %q",
				index, codes[index], setters[index], wantCodes[index], wantSetters[index])
		}
	}
	if len(fds) != 2 {
		t.Fatalf("create_listener carried %d descriptors, want 2 (listen_fd, close_fd)", len(fds))
	}

	// The first descriptor must be a listening socket on the app's path: this is what a
	// compositor would accept on, so a client connecting to the derived socket has to get
	// through it.
	listenFile := os.NewFile(uintptr(fds[0]), "listen_fd")
	defer listenFile.Close()
	handed, err := net.FileListener(listenFile)
	if err != nil {
		t.Fatalf("the descriptor handed over is not a listener: %v", err)
	}
	defer handed.Close()
	accepted := make(chan error, 1)
	go func() {
		conn, err := handed.Accept()
		if err == nil {
			conn.Close()
		}
		accepted <- err
	}()
	client, err := net.Dial("unix", want)
	if err != nil {
		t.Fatalf("a client could not reach the app's socket: %v", err)
	}
	client.Close()
	select {
	case err := <-accepted:
		if err != nil {
			t.Fatalf("accept on the handed-over descriptor: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a connection to the app's socket never arrived on the handed-over descriptor")
	}

	// The second descriptor is the revocation signal. It must stay silent while the holder
	// lives and hang up the moment it closes its end - that ordering IS the mechanism.
	closeFile := os.NewFile(uintptr(fds[1]), "close_fd")
	defer closeFile.Close()
	hangup := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := closeFile.Read(buf)
		hangup <- err
	}()
	select {
	case err := <-hangup:
		t.Fatalf("close_fd signalled before the holder let go: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	lis.close()
	select {
	case err := <-hangup:
		if err == nil {
			t.Fatal("close_fd produced data instead of a hangup")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("closing the holder's end did not hang up close_fd, so nothing would ever revoke the context")
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the socket should be gone after the holder finishes, got %v", err)
	}
}

// A compositor without the manager global is the mainstream case this must survive: it is not
// an error to be reported at the user, it is a fallback the launch takes deliberately.
func TestCreateReportsUnsupported(t *testing.T) {
	srv := startFakeCompositor(t, false, "wl_compositor", "wl_seat")
	runtimeDir := t.TempDir()
	addr := paths.Address{App: "browser"}
	opt := options.HostOptions{RuntimeDir: runtimeDir, WaylandDisplay: srv.path}

	_, err := create(addr, opt)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
	// Nothing may be left behind: the launch is about to mount the compositor's own socket,
	// and a stray one of ours in the runtime dir would outlive every app that ever fell back.
	if _, err := os.Stat(SocketPath(runtimeDir, srv.path, addr)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a socket was left behind after falling back: %v", err)
	}
}

// The commit is answered by a roundtrip precisely so a rejection is seen. Without it the
// launch would carry on believing the app had a context it never got.
func TestCreateSurfacesProtocolError(t *testing.T) {
	srv := startFakeCompositor(t, true, managerInterface)
	addr := paths.Address{App: "browser"}
	opt := options.HostOptions{RuntimeDir: t.TempDir(), WaylandDisplay: srv.path}

	_, err := create(addr, opt)
	if err == nil {
		t.Fatal("a rejected commit must fail the launch")
	}
	var perr protocolError
	if !errors.As(err, &perr) {
		t.Fatalf("want the compositor's own error, got %T: %v", err, err)
	}
	if perr.code != 3 || perr.message != "instance id is not unique" {
		t.Fatalf("the compositor's reason was lost: %+v", perr)
	}
}

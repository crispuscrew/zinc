package audio

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRXMatchesUpstream(t *testing.T) {
	if PermissionRX != 0400|0100 {
		t.Fatal("PipeWire permission ABI differs")
	}
}

func TestDecoderRejectsInvalidDictionaryCounts(t *testing.T) {
	for _, count := range []uint32{0xffffffff, 4097, 1} {
		read := &reader{data: structure(integer(count))}
		if _, err := read.dict(); err == nil {
			t.Fatalf("accepted dictionary count %d", count)
		}
	}
	read := &reader{data: dictionary("duplicate", "one", "duplicate", "two")}
	if _, err := read.dict(); err == nil {
		t.Fatal("accepted duplicate identity key")
	}
}

func TestDecoderRejectsTruncation(t *testing.T) {
	encoded := structure(dictionary("node.name", "exact"))
	for length := 0; length < len(encoded); length++ {
		if _, err := fields(encoded[:length]); err == nil {
			t.Fatalf("accepted %d bytes", length)
		}
	}
}

func socketPair(t *testing.T) (*connection, *net.UnixConn) {
	t.Helper()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(t.TempDir(), "socket")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	conn, err := connect(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	peer, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.socket.Close(); peer.Close() })
	return conn, peer
}

func TestReceivePreservesPartialFrameAcrossDeadline(t *testing.T) {
	conn, peer := socketPair(t)
	header := make([]byte, 16)
	wire.PutUint32(header, 1)
	wire.PutUint32(header[4:], 24)
	if _, err := peer.Write(header[:9]); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.recv(time.Now().Add(10 * time.Millisecond)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal(err)
	}
	if _, err := peer.Write(append(header[9:], structure(integer(7))...)); err != nil {
		t.Fatal(err)
	}
	msg, err := conn.recv(time.Now().Add(time.Second))
	if err != nil || msg.object != 1 || len(msg.body) != 24 {
		t.Fatalf("%+v %v", msg, err)
	}
}

func TestReceiveRejectsOversizeHeader(t *testing.T) {
	conn, peer := socketPair(t)
	header := make([]byte, 16)
	wire.PutUint32(header[4:], maxFrame+1)
	if _, err := peer.Write(header); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.recv(time.Now().Add(time.Second)); err == nil {
		t.Fatal("unbounded frame accepted")
	}
}

func TestMissingPolicyCannotReportReady(t *testing.T) {
	conn, _ := socketPair(t)
	if _, err := awaitReady(conn, time.Now().Add(10*time.Millisecond)); err == nil {
		t.Fatal("missing policy accepted")
	}
}

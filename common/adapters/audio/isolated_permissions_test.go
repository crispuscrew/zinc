package audio

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestIsolatedDaemonDeniesKnownHostObjectID(t *testing.T) {
	environment := startIsolated(t, true)
	request := isolatedRequest(environment.runtime)
	session, err := Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	identifier := hostNodeID(t, environment.runtime+"/pipewire-0-manager", "forbidden.sink")
	conn, err := connect(session.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.socket.Close()
	if err := conn.hello("application.name", "direct-protocol-attacker"); err != nil {
		t.Fatal(err)
	}
	if err := conn.send(0, 5, structure(integer(3), integer(2))); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	if err := conn.sync(deadline); err != nil {
		t.Fatal(err)
	}
	if err := conn.send(2, 1, structure(integer(identifier), text("PipeWire:Interface:Node"), integer(3), integer(3))); err != nil {
		t.Fatal(err)
	}
	for {
		_, err := conn.receive(deadline)
		if err != nil {
			if !strings.Contains(err.Error(), "permission") && !strings.Contains(err.Error(), "no global") {
				t.Fatalf("expected daemon access denial, got %v", err)
			}
			break
		}
	}
}

func hostNodeID(t *testing.T, path, name string) uint32 {
	t.Helper()
	conn, err := connect(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.socket.Close()
	if err := conn.hello("application.name", "trusted-test-discovery"); err != nil {
		t.Fatal(err)
	}
	if err := conn.send(0, 5, structure(integer(3), integer(2))); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		msg, err := conn.receive(deadline)
		if err != nil {
			t.Fatal(err)
		}
		if msg.object != 2 || msg.opcode != 0 {
			continue
		}
		read, err := fields(msg.body)
		if err != nil {
			t.Fatal(err)
		}
		identifier, err := read.number()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := read.number(); err != nil {
			t.Fatal(err)
		}
		if _, err := read.string(); err != nil {
			t.Fatal(err)
		}
		if _, err := read.number(); err != nil {
			t.Fatal(err)
		}
		props, err := read.dict()
		if err != nil {
			t.Fatal(err)
		}
		if props["node.name"] == name {
			return identifier
		}
	}
}

package audio

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestIsolatedAllDirectionsAndDefaults(t *testing.T) {
	environment := startIsolated(t, true)
	request := isolatedRequest(environment.runtime)
	request.Audio = schema.AudioMeta{
		Playback:   schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"synthetic.sink", "forbidden.sink"}},
		Microphone: schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"synthetic.source"}},
		Monitor:    schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"synthetic.sink"}},
	}
	session, err := Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	objects := inspectSocket(t, session.Socket)
	for _, endpoint := range session.Endpoints {
		if !objects[endpoint.Name] {
			t.Fatalf("missing %s endpoint %s", endpoint.Direction, endpoint.Name)
		}
	}
}

func TestIsolatedPolicyExitRevokesSession(t *testing.T) {
	environment := startIsolated(t, true)
	session, err := Prepare(context.Background(), isolatedRequest(environment.runtime))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := syscall.Kill(-environment.policy.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Done():
		if session.Err() == nil {
			t.Fatal("lost policy not reported")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("policy loss did not revoke socket")
	}
}

func TestIsolatedTargetRemovalRevokesExistingClient(t *testing.T) {
	environment := startIsolated(t, true)
	session, err := Prepare(context.Background(), isolatedRequest(environment.runtime))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	client, err := connect(session.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.socket.Close()
	if err := client.hello("application.name", "existing-client"); err != nil {
		t.Fatal(err)
	}
	removeTestNode(t, environment.runtime+"/pipewire-0-manager", "synthetic.sink")
	select {
	case <-session.Done():
		if session.Err() == nil {
			t.Fatal("target loss not reported")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("target loss did not revoke socket")
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := client.receive(deadline); err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatal("existing client was not disconnected")
			}
			break
		}
	}
}

func removeTestNode(t *testing.T, path, name string) {
	t.Helper()
	conn, err := connect(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.socket.Close()
	if err := conn.hello("application.name", "synthetic-node-removal"); err != nil {
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
		if props["node.name"] != name {
			continue
		}
		if err := conn.send(2, 2, structure(integer(identifier))); err != nil {
			t.Fatal(err)
		}
		if err := conn.sync(deadline); err != nil {
			t.Fatal(err)
		}
		return
	}
}

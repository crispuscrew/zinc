package audio

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func isolatedRequest(runtime string) Request {
	return Request{RuntimeDir: runtime, AppID: "test", InstanceID: "test.one",
		Audio: schema.AudioMeta{Playback: schema.AudioDevice{PipeWireDevices: []string{"synthetic.sink"}}}}
}

func TestIsolatedMissingPolicyAndInitialGate(t *testing.T) {
	environment := startIsolated(t, false)
	request := isolatedRequest(environment.runtime)
	request.Timeout = 300 * time.Millisecond
	if _, err := Prepare(context.Background(), request); err == nil {
		t.Fatal("missing policy accepted")
	}
	session := newSession()
	if err := session.createContext(request, "unregistered", time.Now().Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	defer session.finish(nil)
	if objects := inspectSocket(t, session.Socket); len(objects) != 0 {
		t.Fatalf("ungated initial graph: %v", objects)
	}
}

func TestIsolatedNamedSelectionMultipleConnectionsAndClose(t *testing.T) {
	environment := startIsolated(t, true)
	request := isolatedRequest(environment.runtime)
	session, err := Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for attempt := 0; attempt < 3; attempt++ {
		objects := inspectSocket(t, session.Socket)
		if !objects[session.Endpoints[0].Name] {
			t.Fatalf("approved endpoint absent: %v", objects)
		}
		for name := range objects {
			if strings.Contains(name, "synthetic") || strings.Contains(name, "forbidden") || strings.HasSuffix(name, ".bridge") {
				t.Fatalf("host graph exposed: %s", name)
			}
		}
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(session.Socket); !os.IsNotExist(err) {
		t.Fatalf("socket not revoked: %v", err)
	}
}

func TestIsolatedMissingNamedTargetFails(t *testing.T) {
	environment := startIsolated(t, true)
	request := isolatedRequest(environment.runtime)
	request.Audio.Playback.PipeWireDevices = []string{"not-present"}
	_, err := Prepare(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing target not reported: %v", err)
	}
}

func inspectSocket(t *testing.T, path string) map[string]bool {
	t.Helper()
	conn, err := connect(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.socket.Close()
	if err := conn.hello("application.name", "untrusted-probe", "pipewire.access", "unrestricted",
		"pipewire.sec.engine", "forged", "pipewire.client.access", "unrestricted", "media.category", "Manager",
		"zinc.audio.protocol", PolicyVersion, "zinc.audio.request", "{}"); err != nil {
		t.Fatal(err)
	}
	if err := conn.send(0, 5, structure(integer(3), integer(2))); err != nil {
		t.Fatal(err)
	}
	objects := map[string]bool{}
	deadline := time.Now().Add(800 * time.Millisecond)
	for {
		msg, err := conn.receive(deadline)
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return objects
		}
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
		_, err = read.number()
		if err != nil {
			t.Fatal(err)
		}
		_, err = read.number()
		if err != nil {
			t.Fatal(err)
		}
		_, err = read.string()
		if err != nil {
			t.Fatal(err)
		}
		_, err = read.number()
		if err != nil {
			t.Fatal(err)
		}
		props, err := read.dict()
		if err != nil {
			t.Fatal(err)
		}
		if name := props["node.name"]; name != "" {
			objects[name] = true
		}
	}
}

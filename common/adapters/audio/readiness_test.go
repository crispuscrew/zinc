package audio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestPolicyRefusalAndVersionMismatch(t *testing.T) {
	for _, properties := range [][]string{
		{"zinc.audio.status", "error", "zinc.audio.error", "named node unavailable"},
		{"zinc.audio.status", "ready", "zinc.audio.version", "other"},
	} {
		conn, peer := socketPair(t)
		server := &connection{socket: peer}
		body := structure(integer(10), pod(5, make([]byte, 8)), dictionary(properties...))
		if err := server.send(1, 0, body); err != nil {
			t.Fatal(err)
		}
		if _, err := awaitReady(conn, time.Now().Add(time.Second)); err == nil {
			t.Fatal("invalid readiness accepted")
		}
	}
}

func TestPreparationRejectsPublicRuntimeAndSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	request := Request{RuntimeDir: root, AppID: "test", InstanceID: "one",
		Audio: schema.AudioMeta{Playback: schema.AudioDevice{PipeWireDefault: true}}}
	if _, err := Prepare(context.Background(), request); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "runtime")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	request.RuntimeDir = link
	if _, err := Prepare(context.Background(), request); err == nil {
		t.Fatal("symlinked runtime accepted")
	}
}

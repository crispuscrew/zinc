package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func sampleCommand(t *testing.T, environment isolated, remote, target string, capture bool) (*exec.Cmd, *bytes.Buffer, context.CancelFunc) {
	t.Helper()
	config := filepath.Join(t.TempDir(), "alsa.conf")
	if err := os.WriteFile(config, []byte("pcm.test { type pipewire }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	binaryName := "aplay"
	if capture {
		binaryName = "arecord"
	}
	command := exec.CommandContext(ctx, binaryName, "-q", "-D", "test", "-t", "raw", "-f", "S16_LE", "-r", "48000", "-c", "2", "-d", "1")
	for _, entry := range environment.env {
		if !strings.HasPrefix(entry, "PIPEWIRE_REMOTE=") && !strings.HasPrefix(entry, "PIPEWIRE_CONFIG_DIR=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "PIPEWIRE_REMOTE="+remote, "PIPEWIRE_NODE="+target, "ALSA_CONFIG_PATH="+config)
	output, diagnostic := &bytes.Buffer{}, &bytes.Buffer{}
	command.Stdout, command.Stderr = output, diagnostic
	if capture {
		command.Env = append(command.Env, `PIPEWIRE_PROPS={ stream.capture.sink = true }`)
	}
	if !capture {
		samples := make([]byte, 48000*4)
		for offset := 0; offset < len(samples); offset += 2 {
			binary.LittleEndian.PutUint16(samples[offset:], 4096)
		}
		command.Stdin = bytes.NewReader(samples)
	}
	t.Cleanup(func() {
		cancel()
		if t.Failed() {
			t.Log(diagnostic.String())
		}
	})
	return command, output, cancel
}

func TestIsolatedMonitorSignalAndPlaybackCaptureDenial(t *testing.T) {
	environment := startIsolated(t, true)
	request := isolatedRequest(environment.runtime)
	request.Audio = schema.AudioMeta{Monitor: schema.AudioDevice{PipeWireDevices: []string{"synthetic.sink"}}}
	monitor, err := Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer monitor.Close()
	recorder, samples, cancel := sampleCommand(t, environment, monitor.Socket, monitor.Endpoints[0].Name, true)
	// This endpoint is a source, not a sink monitor; the host bridge does monitor capture.
	recorder.Env = append(recorder.Env, `PIPEWIRE_PROPS={ stream.capture.sink = false }`)
	if err := recorder.Start(); err != nil {
		t.Fatal(err)
	}
	player, _, _ := sampleCommand(t, environment, "pipewire-0", "synthetic.sink", false)
	time.Sleep(200 * time.Millisecond)
	if err := player.Run(); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Wait(); err != nil {
		t.Fatalf("monitor recording: %v", err)
	}
	cancel()
	if !hasSignal(samples.Bytes()) {
		t.Fatal("approved monitor delivered no synthetic signal")
	}
	playback, err := Prepare(context.Background(), isolatedRequest(environment.runtime))
	if err != nil {
		t.Fatal(err)
	}
	defer playback.Close()
	denied, data, _ := sampleCommand(t, environment, playback.Socket, "synthetic.sink", true)
	if err := denied.Start(); err != nil {
		t.Fatal(err)
	}
	player, _, _ = sampleCommand(t, environment, "pipewire-0", "synthetic.sink", false)
	if err := player.Run(); err != nil {
		t.Fatal(err)
	}
	_ = denied.Wait() // denied capture can fail or remain inactive until timeout
	if hasSignal(data.Bytes()) {
		t.Fatal("playback-only client captured host monitor")
	}
}

func TestIsolatedPlaybackDeliversSignal(t *testing.T) {
	environment := startIsolated(t, true)
	session, err := Prepare(context.Background(), isolatedRequest(environment.runtime))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	recorder, samples, _ := sampleCommand(t, environment, "pipewire-0", "synthetic.sink", true)
	if err := recorder.Start(); err != nil {
		t.Fatal(err)
	}
	player, _, _ := sampleCommand(t, environment, session.Socket, session.Endpoints[0].Name, false)
	time.Sleep(200 * time.Millisecond)
	if err := player.Run(); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Wait(); err != nil {
		t.Fatal(err)
	}
	if !hasSignal(samples.Bytes()) {
		t.Fatal("approved playback delivered no signal")
	}
}

func hasSignal(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return true
		}
	}
	return false
}

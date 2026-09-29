package audioholder

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	shared "github.com/crispuscrew/zinc/common/adapters/audio"
	plan "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

type fakeSession struct {
	ready Ready
	done  chan struct{}
	once  sync.Once
}

func (value *fakeSession) Description() Ready    { return value.ready }
func (value *fakeSession) Done() <-chan struct{} { return value.done }
func (value *fakeSession) Err() error            { return nil }
func (value *fakeSession) Close() error          { value.once.Do(func() { close(value.done) }); return nil }

func TestMain(check *testing.M) {
	if mode := os.Getenv("ZINC_TEST_AUDIO_HOLDER"); mode != "" {
		err := hold(os.Args[2], os.Args[3], func(_ context.Context, request shared.Request) (session, error) {
			if mode == "reject" {
				return nil, fmt.Errorf("policy refused requested device")
			}
			if !request.Audio.Playback.PipeWireDefault || len(request.Audio.Playback.PipeWireDevices) != 1 ||
				!request.Audio.Monitor.PipeWireDefault || len(request.Audio.Microphone.PipeWireDevices) != 1 {
				return nil, fmt.Errorf("full directional request was not transported")
			}
			grants, err := plan.Build(request.Audio)
			if err != nil {
				return nil, err
			}
			socket := filepath.Join(request.RuntimeDir, "za-fixture", "pipewire-0")
			ready := Ready{Audio: qemu.AudioLayout{Socket: socket, ALSA: grants.ALSA}, Environment: []string{"PIPEWIRE_REMOTE=" + socket}}
			for index, selection := range grants.PipeWire {
				ready.Audio.Endpoints = append(ready.Audio.Endpoints, qemu.AudioEndpoint{Direction: selection.Direction, Name: fmt.Sprintf("za.fixture.%d", index)})
			}
			value := &fakeSession{ready: ready, done: make(chan struct{})}
			if mode == "revoke" {
				time.AfterFunc(20*time.Millisecond, func() { value.Close() })
			}
			return value, nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(check.Run())
}

func holderRequest(check *testing.T) shared.Request {
	return shared.Request{RuntimeDir: check.TempDir(), AppID: "guest", InstanceID: "launch", Audio: schema.AudioMeta{
		Playback:   schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"host.sink"}},
		Microphone: schema.AudioDevice{PipeWireDevices: []string{"host.source"}}, Monitor: schema.AudioDevice{PipeWireDefault: true},
	}}
}

func TestDetachedHolderReadinessAndGracefulLifetime(check *testing.T) {
	check.Setenv("ZINC_TEST_AUDIO_HOLDER", "ready")
	handle, err := Start(holderRequest(check), os.Stderr)
	if err != nil {
		check.Fatal(err)
	}
	if len(handle.Ready.Audio.Endpoints) != 4 {
		check.Fatal("lost additive or monitor grants")
	}
	select {
	case <-handle.Done:
		check.Fatal("holder exited before guest lifetime ended")
	default:
	}
	if err := handle.Close(); err != nil {
		check.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		check.Fatal("close not idempotent:", err)
	}
}

func TestHolderFailureNeverReturnsHostSocket(check *testing.T) {
	check.Setenv("ZINC_TEST_AUDIO_HOLDER", "reject")
	if handle, err := Start(holderRequest(check), io.Discard); err == nil || handle != nil {
		check.Fatal("failed policy returned usable audio")
	}
}

func TestHolderRevocationIsObservable(check *testing.T) {
	check.Setenv("ZINC_TEST_AUDIO_HOLDER", "revoke")
	handle, err := Start(holderRequest(check), io.Discard)
	if err != nil {
		check.Fatal(err)
	}
	defer handle.Close()
	select {
	case <-handle.Done:
	case <-time.After(3 * time.Second):
		check.Fatal("revocation not reported")
	}
}

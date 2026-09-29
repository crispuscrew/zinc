// Package audioholder keeps the shared audio broker in a detached helper process.
package audioholder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	shared "github.com/crispuscrew/zinc/common/adapters/audio"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/ipc"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

const Command = "__audio-holder"

type Ready struct {
	Error       string
	Audio       qemu.AudioLayout
	Environment []string
}

type session interface {
	Description() Ready
	Done() <-chan struct{}
	Err() error
	Close() error
}

type brokerSession struct{ *shared.Session }

func (value brokerSession) Description() Ready {
	ready := Ready{Audio: qemu.AudioLayout{Socket: value.Socket, ALSA: value.ALSA}, Environment: value.Environment()}
	for _, endpoint := range value.Endpoints {
		ready.Audio.Endpoints = append(ready.Audio.Endpoints, qemu.AudioEndpoint{Direction: endpoint.Direction, Name: endpoint.Name})
	}
	return ready
}

func prepare(ctx context.Context, request shared.Request) (session, error) {
	value, err := shared.Prepare(ctx, request)
	if err != nil {
		return nil, err
	}
	return brokerSession{value}, nil
}

func Hold(name, instance string) error {
	return hold(name, instance, prepare)
}

func hold(name, instance string, start func(context.Context, shared.Request) (session, error)) error {
	status, input, lifetime, err := ipc.Inherited()
	if err != nil {
		return err
	}
	defer status.Close()
	defer input.Close()
	defer lifetime.Close()
	var request shared.Request
	if err := ipc.Receive(input, &request); err != nil {
		_ = ipc.Send(status, Ready{Error: err.Error()})
		return err
	}
	if request.AppID != name || request.InstanceID != instance {
		err := fmt.Errorf("audio holder identity differs from inherited request")
		_ = ipc.Send(status, Ready{Error: err.Error()})
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	return serve(ctx, request, status, lifetime, start)
}

func serve(ctx context.Context, request shared.Request, status *os.File, lifetime io.Reader,
	start func(context.Context, shared.Request) (session, error)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _, _ = io.Copy(io.Discard, lifetime); cancel() }()
	value, err := start(ctx, request)
	if err != nil {
		_ = ipc.Send(status, Ready{Error: err.Error()})
		return err
	}
	defer value.Close()
	ready := value.Description()
	if ready.Audio.Socket == "" {
		return fmt.Errorf("audio holder requires PipeWire grants")
	}
	if err := ipc.Send(status, ready); err != nil {
		return err
	}
	status.Close()
	select {
	case <-ctx.Done():
		err := value.Close()
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-value.Done():
		return value.Err()
	}
}

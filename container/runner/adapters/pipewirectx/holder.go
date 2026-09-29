package pipewirectx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	shared "github.com/crispuscrew/zinc/common/adapters/audio"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
)

const maxRequest = 64 << 10

// Hold keeps the broker alive until the container disappears or policy fails.
// The legacy boolean cannot grant access without the complete inherited request.
func Hold(addr paths.Address, _ bool, opt options.HostOptions, wait func(string) error) error {
	status := os.NewFile(3, "audio-readiness")
	defer status.Close()
	input := os.NewFile(4, "audio-request")
	defer input.Close()
	request, err := decodeRequest(input)
	if err == nil && (request.AppID != addr.App || request.InstanceID != addr.Runtime() || request.RuntimeDir != opt.RuntimeDir) {
		err = fmt.Errorf("audio: holder identity does not match inherited request")
	}
	if err != nil {
		fmt.Fprintln(status, "error", err)
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	session, err := shared.Prepare(ctx, request)
	if err != nil {
		fmt.Fprintln(status, "error", err)
		return err
	}
	defer session.Close()
	if session.Socket == "" {
		return fmt.Errorf("audio: holder received no PipeWire grants")
	}
	if _, err := fmt.Fprintln(status, "ok", session.Socket); err != nil {
		return err
	}
	status.Close()
	finished := make(chan error, 1)
	go func() { finished <- wait(addr.Runtime()) }()
	select {
	case err := <-finished:
		return errors.Join(err, session.Close())
	case <-session.Done():
		return session.Err()
	}
}

func decodeRequest(input io.Reader) (shared.Request, error) {
	var request shared.Request
	body, err := io.ReadAll(io.LimitReader(input, maxRequest+1))
	if err != nil {
		return request, err
	}
	if len(body) > maxRequest {
		return request, fmt.Errorf("audio: holder request too large")
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return request, fmt.Errorf("audio: inherited request: %w", err)
	}
	return request, nil
}

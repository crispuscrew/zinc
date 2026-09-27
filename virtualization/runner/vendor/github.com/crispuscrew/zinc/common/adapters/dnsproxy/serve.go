package dnsproxy

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

const maxHandlers = 64

type proxy struct {
	resolver  *Resolver
	status    status
	ready     atomic.Bool
	packets   []net.PacketConn
	listeners []net.Listener
	control   *controlListener
	handlers  chan struct{}
	workers   sync.WaitGroup
}

// Serve binds UDP and TCP at every explicit address, then publishes Unix status.
// Bare IPs use port 53. Port zero is for isolated tests; status reports actual ports.
// Cancellation closes listeners and in-flight operations and returns nil.
func Serve(ctx context.Context, meta schema.DNSMeta, addresses []string, controlSocket string) error {
	if ctx.Err() != nil {
		return nil
	}
	runtime, err := bind(meta, addresses, controlSocket)
	if err != nil {
		return err
	}
	return runtime.run(ctx)
}

func bind(meta schema.DNSMeta, addresses []string, controlSocket string) (*proxy, error) {
	resolver, err := New(meta)
	if err != nil {
		return nil, err
	}
	addresses, err = listenerAddresses(addresses)
	if err != nil {
		return nil, err
	}
	runtime := &proxy{resolver: resolver, handlers: make(chan struct{}, maxHandlers)}
	runtime.status = status{Version: 1, Digest: configDigest(meta)}
	complete := false
	defer func() {
		if !complete {
			runtime.close()
		}
	}()
	for _, address := range addresses {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return nil, err
		}
		runtime.listeners = append(runtime.listeners, listener)
		actual := listener.Addr().String()
		packet, err := net.ListenPacket("udp", actual)
		if err != nil {
			return nil, err
		}
		runtime.packets = append(runtime.packets, packet)
		runtime.status.Addresses = append(runtime.status.Addresses, actual)
	}
	runtime.control, err = listenControl(controlSocket)
	if err != nil {
		return nil, err
	}
	complete = true
	return runtime, nil
}

func (runtime *proxy) run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	failures := make(chan error, len(runtime.listeners)*2+1)
	var loops sync.WaitGroup
	launch := func(serve func() error) {
		loops.Add(1)
		go func() { defer loops.Done(); failures <- serve() }()
	}
	for _, packet := range runtime.packets {
		launch(func() error { return runtime.serveUDP(ctx, packet) })
	}
	for _, listener := range runtime.listeners {
		launch(func() error { return runtime.serveTCP(ctx, listener) })
	}
	launch(func() error { return runtime.serveControl(ctx) })
	runtime.ready.Store(ctx.Err() == nil)
	var err error
	select {
	case <-ctx.Done():
	case err = <-failures:
	}
	runtime.ready.Store(false)
	cancel()
	closeErr := runtime.close()
	loops.Wait()
	runtime.workers.Wait()
	return errors.Join(err, closeErr)
}

func (runtime *proxy) close() error {
	for _, listener := range runtime.listeners {
		listener.Close()
	}
	for _, packet := range runtime.packets {
		packet.Close()
	}
	if runtime.control != nil {
		return runtime.control.close()
	}
	return nil
}

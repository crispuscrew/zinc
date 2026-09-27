package dnsproxy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"golang.org/x/sys/unix"
)

type status struct {
	Version   int      `json:"version"`
	Digest    string   `json:"digest"`
	Addresses []string `json:"addresses"`
	Ready     bool     `json:"ready"`
}

func configDigest(meta schema.DNSMeta) string {
	encoded, _ := json.Marshal(meta) // DNSMeta contains only JSON-supported values.
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func checkPeer(connection *net.UnixConn) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return err
	}
	var credentials *unix.Ucred
	var peerErr error
	if err := raw.Control(func(descriptor uintptr) {
		credentials, peerErr = unix.GetsockoptUcred(int(descriptor), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if peerErr != nil {
		return peerErr
	}
	if credentials == nil || !trustedUID(credentials.Uid) {
		return fmt.Errorf("untrusted DNS control peer")
	}
	return nil
}

func (runtime *proxy) serveControl(ctx context.Context) error {
	for {
		connection, err := runtime.control.listener.AcceptUnix()
		if err != nil {
			return err
		}
		// Status needs no request body and is served serially, without new handlers.
		func() {
			defer connection.Close()
			stop := context.AfterFunc(ctx, func() { connection.Close() })
			defer stop()
			if checkPeer(connection) != nil {
				return
			}
			reply := runtime.status
			reply.Ready = runtime.ready.Load() && ctx.Err() == nil
			encoded, err := json.Marshal(reply)
			if err == nil {
				_ = writeStatus(connection, encoded) // A disconnected status client is disposable.
			}
		}()
	}
}

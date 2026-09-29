// Package audio brokers instance audio through the versioned Zinc WirePlumber policy.
package audio

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	plan "github.com/crispuscrew/zinc/common/domain/audio"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

const Engine = "com.github.crispuscrew.zinc"
const Access = "zinc-audio-v1"
const PolicyVersion = "1"

type Request struct {
	RuntimeDir string
	AppID      string
	InstanceID string
	Audio      schema.AudioMeta
	// Timeout bounds preparation, not session lifetime. Zero means ten seconds.
	Timeout time.Duration
}

// Endpoint is a private node.name usable with QEMU in.name or out.name.
// Target is the selected host node.name, for host-side reporting only.
type Endpoint struct {
	Direction plan.Direction `json:"direction"`
	Name      string         `json:"name"`
	Target    string         `json:"target"`
}

// Session must outlive the container or QEMU process. Close is idempotent.
// A lifetime failure closes Done, revokes the socket and is returned by Err.
type Session struct {
	Socket      string
	Endpoints   []Endpoint
	ALSA        []plan.PCM
	controller  *connection
	context     *connection
	listener    *os.File
	revokeRead  *os.File
	revokeWrite *os.File
	directory   string
	done        chan struct{}
	stop        chan struct{}
	once        sync.Once
	lock        sync.Mutex
	err         error
}

func (session *Session) Done() <-chan struct{} { return session.done }

func (session *Session) Err() error {
	session.lock.Lock()
	defer session.lock.Unlock()
	return session.err
}

func (session *Session) Close() error {
	session.once.Do(func() { close(session.stop) })
	<-session.done
	return session.Err()
}

func (session *Session) finish(cause error) {
	// Revoke first; controller disconnect then tells the policy to destroy bridges.
	for _, handle := range []*os.File{session.revokeWrite, session.revokeRead, session.listener} {
		if handle != nil {
			cause = errors.Join(cause, handle.Close())
		}
	}
	for _, conn := range []*connection{session.context, session.controller} {
		if conn != nil {
			cause = errors.Join(cause, conn.socket.Close())
		}
	}
	if session.Socket != "" {
		if err := os.Remove(session.Socket); err != nil && !os.IsNotExist(err) {
			cause = errors.Join(cause, err)
		}
	}
	if session.directory != "" {
		cause = errors.Join(cause, os.Remove(session.directory))
	}
	session.lock.Lock()
	session.err = cause
	session.lock.Unlock()
	close(session.done)
}

func newSession() *Session { return &Session{done: make(chan struct{}), stop: make(chan struct{})} }

func (session *Session) Environment() []string {
	if session.Socket == "" {
		return nil
	}
	return []string{"PIPEWIRE_REMOTE=" + session.Socket}
}

func deadlineFor(ctx context.Context, timeout time.Duration) time.Time {
	end := time.Now().Add(timeout)
	if deadline, exists := ctx.Deadline(); exists && deadline.Before(end) {
		return deadline
	}
	return end
}

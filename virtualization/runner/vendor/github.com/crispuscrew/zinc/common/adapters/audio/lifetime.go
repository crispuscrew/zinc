package audio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const heartbeatInterval = time.Second
const heartbeatTimeout = 4 * time.Second

func (session *Session) supervise(ctx context.Context) {
	var cause error
	defer func() { session.finish(cause) }()
	lastReply, lastPing := time.Now(), time.Time{}
	var sequence uint64
	var expected string
	for {
		select {
		case <-session.stop:
			return
		case <-ctx.Done():
			cause = ctx.Err()
			return
		default:
		}
		if time.Since(lastReply) > heartbeatTimeout {
			cause = fmt.Errorf("audio: policy heartbeat lost; socket revoked")
			return
		}
		if time.Since(lastPing) >= heartbeatInterval {
			sequence++
			expected = strconv.FormatUint(sequence, 10)
			if err := session.controller.properties("zinc.audio.ping", expected); err != nil {
				cause = err
				return
			}
			lastPing = time.Now()
		}
		msg, err := session.controller.receive(time.Now().Add(250 * time.Millisecond))
		if errors.Is(err, os.ErrDeadlineExceeded) {
			continue
		}
		if err != nil {
			cause = err
			return
		}
		if msg.object != 1 || msg.opcode != 0 {
			continue
		}
		props, err := clientProperties(msg.body)
		if err != nil {
			cause = err
			return
		}
		if err := policyError(props); err != nil {
			cause = err
			return
		}
		if props["zinc.audio.pong"] == expected && props["zinc.audio.version"] == PolicyVersion {
			lastReply = time.Now()
		}
	}
}

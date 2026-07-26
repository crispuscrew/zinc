// Package qmp is a minimal client for QEMU's Machine Protocol, the JSON control channel
// on a running guest's socket. zvr uses it for the two things supervision needs: asking a
// guest what state it is in, and pressing its virtual power button so the guest shuts
// down the way it would on real hardware rather than being killed mid-write.
//
// Only the handful of commands zvr issues are implemented. A full QMP client would be a
// dependency; this is a hundred lines of encoding/json over a unix socket, and it keeps
// the runner free of anything that is not the standard library.
package qmp

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// dialTimeout bounds the connect. A socket that exists but never answers means a guest
// that died without cleaning up, and zvr should say so rather than hang.
const dialTimeout = 3 * time.Second

// Conn is one QMP session. Not safe for concurrent use: each command writes a request and
// reads until its reply, so two callers would interleave.
type Conn struct {
	conn    net.Conn
	decoder *json.Decoder
}

// Dial connects to a guest's QMP socket and completes the capabilities handshake, after
// which the connection accepts commands. QMP opens with a greeting and refuses everything
// until the client answers it.
func Dial(socketPath string) (*Conn, error) {
	conn, err := net.DialTimeout("unix", socketPath, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect to the guest's control socket: %w", err)
	}
	session := &Conn{conn: conn, decoder: json.NewDecoder(conn)}

	var greeting struct {
		QMP json.RawMessage `json:"QMP"`
	}
	if err := session.decoder.Decode(&greeting); err != nil {
		session.Close()
		return nil, fmt.Errorf("read the QMP greeting: %w", err)
	}
	if _, err := session.Execute("qmp_capabilities"); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (session *Conn) Close() error { return session.conn.Close() }

// reply is one QMP response: exactly one of return or error is set. Events can arrive
// interleaved with replies, which is why Execute skips anything carrying neither.
type reply struct {
	Return json.RawMessage `json:"return"`
	Error  *struct {
		Class string `json:"class"`
		Desc  string `json:"desc"`
	} `json:"error"`
	Event string `json:"event"`
}

// Execute runs one command and returns its raw result.
func (session *Conn) Execute(command string) (json.RawMessage, error) {
	request, err := json.Marshal(map[string]string{"execute": command})
	if err != nil {
		return nil, err
	}
	if _, err := session.conn.Write(append(request, '\n')); err != nil {
		return nil, fmt.Errorf("send %s: %w", command, err)
	}
	// A guest emits events (SHUTDOWN, RESET, ...) whenever it likes, including between
	// this request and its reply, so read past them rather than mistaking one for the
	// answer.
	for {
		var response reply
		if err := session.decoder.Decode(&response); err != nil {
			return nil, fmt.Errorf("read the reply to %s: %w", command, err)
		}
		switch {
		case response.Error != nil:
			return nil, fmt.Errorf("%s: %s", command, response.Error.Desc)
		case response.Return != nil:
			return response.Return, nil
		case response.Event != "":
			continue
		default:
			return nil, fmt.Errorf("%s: unrecognised QMP reply", command)
		}
	}
}

// Status is what a guest reports about itself.
type Status struct {
	Status  string `json:"status"`  // running, paused, shutdown, ...
	Running bool   `json:"running"` // whether the CPUs are executing
}

// QueryStatus asks the guest what state it is in. A pid alone only proves a process
// exists; this proves the guest inside it is actually running rather than paused or
// wedged mid-shutdown.
func (session *Conn) QueryStatus() (Status, error) {
	raw, err := session.Execute("query-status")
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(raw, &status); err != nil {
		return Status{}, fmt.Errorf("decode query-status: %w", err)
	}
	return status, nil
}

// Powerdown presses the guest's ACPI power button: the guest's own OS sees a shutdown
// request and unmounts its filesystems, which killing the process would not give it the
// chance to do. It returns as soon as the button is pressed - the caller waits for the
// process to go away.
func (session *Conn) Powerdown() error {
	_, err := session.Execute("system_powerdown")
	return err
}

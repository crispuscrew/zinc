package audio

import (
	"fmt"
	"io"
	"net"
	"syscall"
	"time"
)

const maxFrame = 1 << 20
const ioTimeout = 5 * time.Second

type message struct {
	object uint32
	opcode byte
	body   []byte
}
type connection struct {
	socket   *net.UnixConn
	buffer   []byte
	sequence uint32
	next     uint32
}

func connect(path string) (*connection, error) {
	stream, err := net.DialTimeout("unix", path, ioTimeout)
	if err != nil {
		return nil, fmt.Errorf("audio: connect %s: %w", path, err)
	}
	return &connection{socket: stream.(*net.UnixConn), next: 2}, nil
}

func (conn *connection) send(object uint32, opcode byte, body []byte, handles ...int) error {
	if len(body) > maxFrame {
		return errProtocol
	}
	header := make([]byte, 16)
	wire.PutUint32(header, object)
	wire.PutUint32(header[4:], uint32(opcode)<<24|uint32(len(body)))
	wire.PutUint32(header[8:], conn.sequence)
	wire.PutUint32(header[12:], uint32(len(handles)))
	conn.sequence++
	if err := conn.socket.SetWriteDeadline(time.Now().Add(ioTimeout)); err != nil {
		return err
	}
	frame := append(header, body...)
	if len(handles) > 0 {
		written, _, err := conn.socket.WriteMsgUnix(frame, syscall.UnixRights(handles...), nil)
		if err != nil {
			return err
		}
		frame = frame[written:]
	}
	for len(frame) > 0 {
		written, err := conn.socket.Write(frame)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		frame = frame[written:]
	}
	return nil
}

// recv closes unexpected SCM_RIGHTS descriptors, bounds allocations, and preserves
// partial frames across deadline expiry. No stream data/memory objects are bound.
func (conn *connection) recv(deadline time.Time) (message, error) {
	for {
		if len(conn.buffer) >= 16 {
			length := int(wire.Uint32(conn.buffer[4:]) & 0xffffff)
			if length > maxFrame || wire.Uint32(conn.buffer[12:]) != 0 {
				return message{}, errProtocol
			}
			if len(conn.buffer) >= 16+length {
				frame := conn.buffer[:16+length]
				conn.buffer = conn.buffer[16+length:]
				return message{wire.Uint32(frame), frame[7], frame[16:]}, nil
			}
		}
		if err := conn.socket.SetReadDeadline(deadline); err != nil {
			return message{}, err
		}
		chunk, ancillary := make([]byte, 16384), make([]byte, 4096)
		count, control, flags, _, err := conn.socket.ReadMsgUnix(chunk, ancillary)
		if control > 0 {
			messages, parseErr := syscall.ParseSocketControlMessage(ancillary[:control])
			for _, item := range messages {
				handles, _ := syscall.ParseUnixRights(&item)
				for _, handle := range handles {
					syscall.Close(handle)
				}
			}
			if parseErr != nil {
				return message{}, parseErr
			}
			return message{}, fmt.Errorf("audio: unexpected descriptors from control connection")
		}
		if flags&(syscall.MSG_TRUNC|syscall.MSG_CTRUNC) != 0 {
			return message{}, errProtocol
		}
		if count > 0 {
			conn.buffer = append(conn.buffer, chunk[:count]...)
		}
		if err != nil {
			return message{}, err
		}
		if count == 0 {
			return message{}, io.EOF
		}
	}
}

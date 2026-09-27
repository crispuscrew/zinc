package audio

import (
	"fmt"
	"time"
)

const securityInterface = "PipeWire:Interface:SecurityContext"

func (conn *connection) hello(pairs ...string) error {
	// Core v3 avoids the generation-footer protocol added to v4.
	if err := conn.send(0, 1, structure(integer(3))); err != nil {
		return err
	}
	return conn.properties(pairs...)
}

func (conn *connection) properties(pairs ...string) error {
	return conn.send(1, 2, structure(dictionary(pairs...)))
}

func (conn *connection) receive(deadline time.Time) (message, error) {
	for {
		msg, err := conn.recv(deadline)
		if err != nil {
			return message{}, err
		}
		if msg.object != 0 {
			return msg, nil
		}
		switch msg.opcode {
		case 2: // Ping requires Pong with the same object/sequence.
			if err := conn.send(0, 3, msg.body); err != nil {
				return message{}, err
			}
		case 3:
			read, err := fields(msg.body)
			if err != nil {
				return message{}, err
			}
			object, err := read.number()
			if err != nil {
				return message{}, err
			}
			_, err = read.number()
			if err != nil {
				return message{}, err
			}
			code, err := read.number()
			if err != nil {
				return message{}, err
			}
			detail, err := read.string()
			if err != nil {
				return message{}, err
			}
			return message{}, fmt.Errorf("audio: PipeWire object %d: %s (%d)", object, detail, int32(code))
		default:
			return msg, nil
		}
	}
}

func (conn *connection) securityContext(deadline time.Time) (uint32, error) {
	registry := conn.next
	conn.next++
	if err := conn.send(0, 5, structure(integer(3), integer(registry))); err != nil {
		return 0, err
	}
	for {
		msg, err := conn.receive(deadline)
		if err != nil {
			return 0, fmt.Errorf("audio: security-context discovery: %w", err)
		}
		if msg.object != registry || msg.opcode != 0 {
			continue
		}
		read, err := fields(msg.body)
		if err != nil {
			return 0, err
		}
		global, err := read.number()
		if err != nil {
			return 0, err
		}
		_, err = read.number()
		if err != nil {
			return 0, err
		}
		name, err := read.string()
		if err != nil {
			return 0, err
		}
		if name != securityInterface {
			continue
		}
		proxy := conn.next
		conn.next++
		err = conn.send(registry, 1, structure(integer(global), text(name), integer(3), integer(proxy)))
		return proxy, err
	}
}

func (conn *connection) sync(deadline time.Time) error {
	sequence := conn.sequence
	if err := conn.send(0, 2, structure(integer(0), integer(sequence))); err != nil {
		return err
	}
	for {
		msg, err := conn.receive(deadline)
		if err != nil {
			return err
		}
		if msg.object != 0 || msg.opcode != 1 {
			continue
		}
		read, err := fields(msg.body)
		if err != nil {
			return err
		}
		_, err = read.number()
		if err != nil {
			return err
		}
		reply, err := read.number()
		if err != nil {
			return err
		}
		if reply == sequence {
			return nil
		}
	}
}

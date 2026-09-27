// Package ipc provides bounded inherited-pipe messages for private runner helpers.
package ipc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
)

const Maximum = 1 << 20
const ReadyTimeout = 30 * time.Second

func Inherited() (status, request, lifetime *os.File, failure error) {
	for _, descriptor := range []int{3, 4, 5} {
		syscall.CloseOnExec(descriptor)
		// Inherited descriptors lose os.Pipe's poller metadata across exec.
		// Mark them nonblocking before NewFile so deadlines work in the child.
		if err := syscall.SetNonblock(descriptor, true); err != nil {
			return nil, nil, nil, err
		}
	}
	return os.NewFile(3, "readiness"), os.NewFile(4, "request"), os.NewFile(5, "lifetime"), nil
}

func Decode(input io.Reader, target any) error {
	body, err := io.ReadAll(io.LimitReader(input, Maximum+1))
	if err != nil {
		return err
	}
	if len(body) > Maximum {
		return fmt.Errorf("helper message exceeds %d bytes", Maximum)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("helper message contains trailing data")
	}
	return nil
}

func Receive(input *os.File, target any) error {
	return ReceiveWithin(input, target, ReadyTimeout)
}

// ReceiveWithin applies one absolute budget to the entire response. Incoming
// bytes do not extend it. Longer operations must opt in without changing the
// ordinary request, write or acknowledgement timeouts.
func ReceiveWithin(input *os.File, target any, budget time.Duration) error {
	if budget <= 0 {
		return fmt.Errorf("helper receive budget must be positive")
	}
	if err := input.SetReadDeadline(time.Now().Add(budget)); err != nil {
		return err
	}
	return Decode(input, target)
}

func Send(output *os.File, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(body) > Maximum {
		return fmt.Errorf("helper message exceeds %d bytes", Maximum)
	}
	if err := output.SetWriteDeadline(time.Now().Add(ReadyTimeout)); err != nil {
		return err
	}
	_, err = output.Write(body)
	return err
}

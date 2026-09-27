// Package ipc carries bounded messages over inherited descriptors for private helpers.
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

func Inherited() (status, request, activation *os.File, failure error) {
	var files []*os.File
	for _, descriptor := range []int{3, 4, 5} {
		syscall.CloseOnExec(descriptor)
		// os.Pipe's poller metadata does not survive exec; restore deadline support.
		if err := syscall.SetNonblock(descriptor, true); err != nil {
			for _, file := range files {
				file.Close()
			}
			return nil, nil, nil, err
		}
		files = append(files, os.NewFile(uintptr(descriptor), "supervisor-ipc"))
	}
	return files[0], files[1], files[2], nil
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

func Receive(input *os.File, target any, timeout time.Duration) error {
	if err := input.SetReadDeadline(time.Now().Add(timeout)); err != nil {
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

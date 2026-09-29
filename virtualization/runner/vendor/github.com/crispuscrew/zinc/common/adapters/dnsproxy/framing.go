package dnsproxy

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/miekg/dns"
)

func readFrame(reader io.Reader) (*dns.Msg, error) {
	var size uint16
	if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size < 12 {
		return nil, fmt.Errorf("short DNS frame")
	}
	encoded := make([]byte, int(size))
	if _, err := io.ReadFull(reader, encoded); err != nil {
		return nil, err
	}
	message := new(dns.Msg)
	if err := message.Unpack(encoded); err != nil {
		return nil, err
	}
	return message, nil
}

func writeFrame(writer io.Writer, message *dns.Msg) error {
	encoded, err := message.Pack()
	if err != nil {
		return err
	}
	if len(encoded) > dns.MaxMsgSize {
		return fmt.Errorf("DNS frame too large")
	}
	frame := make([]byte, 2, len(encoded)+2)
	binary.BigEndian.PutUint16(frame, uint16(len(encoded)))
	frame = append(frame, encoded...)
	written, err := writer.Write(frame)
	if err == nil && written != len(frame) {
		err = io.ErrShortWrite
	}
	return err
}

package audio

import (
	"bytes"
	"errors"
)

var errProtocol = errors.New("audio: malformed PipeWire message")

type reader struct{ data []byte }

func (read *reader) next(kind uint32) ([]byte, error) {
	if len(read.data) < 8 {
		return nil, errProtocol
	}
	size := uint64(wire.Uint32(read.data))
	length := (size + 7) &^ 7
	if length > uint64(len(read.data)-8) || wire.Uint32(read.data[4:]) != kind {
		return nil, errProtocol
	}
	body := read.data[8 : 8+int(size)]
	read.data = read.data[8+int(length):]
	return body, nil
}

func fields(data []byte) (*reader, error) {
	read := &reader{data: data}
	body, err := read.next(podStruct)
	return &reader{data: body}, err
}

func (read *reader) number() (uint32, error) {
	body, err := read.next(podInt)
	if err != nil || len(body) != 4 {
		return 0, errProtocol
	}
	return wire.Uint32(body), nil
}

func (read *reader) string() (string, error) {
	body, err := read.next(podString)
	if err != nil || len(body) == 0 || body[len(body)-1] != 0 {
		return "", errProtocol
	}
	if bytes.IndexByte(body[:len(body)-1], 0) >= 0 {
		return "", errProtocol
	}
	return string(body[:len(body)-1]), nil
}

func (read *reader) dict() (map[string]string, error) {
	body, err := read.next(podStruct)
	if err != nil {
		return nil, err
	}
	inner := &reader{data: body}
	count, err := inner.number()
	if err != nil || count > 4096 || uint64(count)*32 > uint64(len(inner.data)) {
		return nil, errProtocol
	}
	props := make(map[string]string, count)
	for index := uint32(0); index < count; index++ {
		key, err := inner.string()
		if err != nil {
			return nil, err
		}
		value, err := inner.string()
		if err != nil {
			return nil, err
		}
		if _, exists := props[key]; exists {
			return nil, errProtocol
		}
		props[key] = value
	}
	if len(inner.data) != 0 {
		return nil, errProtocol
	}
	return props, nil
}

func clientProperties(data []byte) (map[string]string, error) {
	read, err := fields(data)
	if err != nil {
		return nil, err
	}
	if _, err := read.number(); err != nil {
		return nil, err
	}
	mask, err := read.next(5) // SPA_TYPE_Long change mask
	if err != nil || len(mask) != 8 {
		return nil, errProtocol
	}
	return read.dict()
}

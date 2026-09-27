package audio

import "encoding/binary"

var wire = binary.LittleEndian

const (
	podInt    uint32 = 4
	podString uint32 = 8
	podStruct uint32 = 14
	podFD     uint32 = 18
	// These are PipeWire masks, not Unix's low permission bits.
	PermissionRX = 0o500
)

func padded(size int) int { return (size + 7) &^ 7 }

func pod(kind uint32, body []byte) []byte {
	result := make([]byte, 8+padded(len(body)))
	wire.PutUint32(result, uint32(len(body)))
	wire.PutUint32(result[4:], kind)
	copy(result[8:], body)
	return result
}

func integer(value uint32) []byte {
	body := make([]byte, 4)
	wire.PutUint32(body, value)
	return pod(podInt, body)
}

func descriptor(index uint64) []byte {
	body := make([]byte, 8)
	wire.PutUint64(body, index)
	return pod(podFD, body)
}

func text(value string) []byte { return pod(podString, append([]byte(value), 0)) }

func structure(fields ...[]byte) []byte {
	var body []byte
	for _, field := range fields {
		body = append(body, field...)
	}
	return pod(podStruct, body)
}

func dictionary(pairs ...string) []byte {
	fields := [][]byte{integer(uint32(len(pairs) / 2))}
	for _, item := range pairs {
		fields = append(fields, text(item))
	}
	return structure(fields...)
}

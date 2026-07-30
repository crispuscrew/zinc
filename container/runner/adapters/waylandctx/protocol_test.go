package waylandctx

import (
	"bytes"
	"errors"
	"testing"
)

// The expectations below are written out as literal bytes rather than rebuilt with the same
// helpers they check, because the encoding is the part that fails silently: a header size
// that excludes itself, or a string that is not padded, does not error anywhere - it shifts
// the stream and the compositor answers about some unrelated object. A test that encoded the
// expectation the same way the code does would agree with any bug.
//
// They assume a little-endian host, which every platform Zinc runs on is. The code itself
// uses the native order (the wire format is host order), so a big-endian build would fail
// here loudly rather than encode differently in silence.

func TestRequestFraming(t *testing.T) {
	// wl_display.get_registry(2): object 1, opcode 1, one new_id argument. Size is 12 and
	// INCLUDES the 8-byte header.
	got := request(displayObject, displayGetRegistry, appendUint32(nil, 2))
	want := []byte{
		0x01, 0x00, 0x00, 0x00, // object id 1 (wl_display)
		0x01, 0x00, 0x0c, 0x00, // opcode 1 in the low half, size 12 in the high half
		0x02, 0x00, 0x00, 0x00, // new_id 2
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("wl_display.get_registry:\n got: % x\nwant: % x", got, want)
	}
}

func TestRequestNoArguments(t *testing.T) {
	// wp_security_context_v1.commit on object 5: header only, size 8.
	got := request(5, contextCommit, nil)
	want := []byte{
		0x05, 0x00, 0x00, 0x00,
		0x04, 0x00, 0x08, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("commit:\n got: % x\nwant: % x", got, want)
	}
}

// A string's length counts the NUL, and the argument is padded to a whole number of 32-bit
// words. Both lengths below are chosen so the padding is visible: "firefox" needs none, "ab"
// needs one byte.
func TestStringArgumentPadding(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  []byte
	}{
		{
			name:  "no padding needed",
			value: "firefox",
			want: []byte{
				0x08, 0x00, 0x00, 0x00, // length 8 = 7 bytes + NUL
				'f', 'i', 'r', 'e', 'f', 'o', 'x', 0x00,
			},
		},
		{
			name:  "padded to the next word",
			value: "ab",
			want: []byte{
				0x03, 0x00, 0x00, 0x00, // length 3 = 2 bytes + NUL
				'a', 'b', 0x00, 0x00, // one byte of padding
			},
		},
		{
			name:  "empty string is still a NUL and a length",
			value: "",
			want: []byte{
				0x01, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := appendString(nil, tc.value)
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("%q:\n got: % x\nwant: % x", tc.value, got, tc.want)
			}
			if len(got)%4 != 0 {
				t.Fatalf("%q encoded to %d bytes, which is not a whole number of words", tc.value, len(got))
			}
		})
	}
}

func TestSetAppIDBytes(t *testing.T) {
	// wp_security_context_v1.set_app_id("firefox") on object 5.
	got := stringRequest(5, contextSetAppID, "firefox")
	want := []byte{
		0x05, 0x00, 0x00, 0x00, // object 5
		0x02, 0x00, 0x14, 0x00, // opcode 2, size 20
		0x08, 0x00, 0x00, 0x00, // string length 8
		'f', 'i', 'r', 'e', 'f', 'o', 'x', 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("set_app_id:\n got: % x\nwant: % x", got, want)
	}
}

// wl_registry.bind is the one request whose new_id is untyped, so the interface name and
// version go on the wire before the id. Getting this wrong binds the right global to an
// object the compositor thinks is a different interface, and the failure surfaces later, on
// some unrelated request.
func TestBindRequestCarriesInterfaceAndVersion(t *testing.T) {
	got := bindRequest(2, 7, "wl_shm", 1, 4)
	want := []byte{
		0x02, 0x00, 0x00, 0x00, // object 2 (the registry)
		0x00, 0x00, 0x20, 0x00, // opcode 0, size 32
		0x07, 0x00, 0x00, 0x00, // global name 7
		0x07, 0x00, 0x00, 0x00, // interface length 7
		'w', 'l', '_', 's', 'h', 'm', 0x00, 0x00, // "wl_shm" + NUL + one pad byte
		0x01, 0x00, 0x00, 0x00, // version 1
		0x04, 0x00, 0x00, 0x00, // new_id 4
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("wl_registry.bind:\n got: % x\nwant: % x", got, want)
	}
}

// create_listener's two descriptors are NOT in the body - they ride as SCM_RIGHTS - so the
// message is a header and the new_id, and nothing else.
func TestCreateListenerBytes(t *testing.T) {
	got := request(4, managerCreateListener, appendUint32(nil, 5))
	want := []byte{
		0x04, 0x00, 0x00, 0x00, // the manager
		0x01, 0x00, 0x0c, 0x00, // opcode 1, size 12
		0x05, 0x00, 0x00, 0x00, // new_id 5
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("create_listener:\n got: % x\nwant: % x", got, want)
	}
}

func TestNextMessageSplitsAndReportsIncomplete(t *testing.T) {
	first := request(1, 0, appendUint32(nil, 42))
	second := stringRequest(5, contextSetInstanceID, "browser.work")
	stream := append(append([]byte(nil), first...), second...)

	// A buffer holding less than one whole message is not an error - it is the ordinary
	// state of a byte stream mid-message.
	for _, short := range []int{0, 4, 7, len(first) - 1} {
		msg, size, err := nextMessage(stream[:short])
		if err != nil || size != 0 || msg.object != 0 {
			t.Fatalf("%d bytes: want (zero, 0, nil), got (%+v, %d, %v)", short, msg, size, err)
		}
	}

	msg, size, err := nextMessage(stream)
	if err != nil {
		t.Fatal(err)
	}
	if size != len(first) || msg.object != 1 || msg.opcode != 0 {
		t.Fatalf("first message: got object %d opcode %d size %d", msg.object, msg.opcode, size)
	}
	msg, size, err = nextMessage(stream[size:])
	if err != nil {
		t.Fatal(err)
	}
	if size != len(second) || msg.object != 5 || msg.opcode != contextSetInstanceID {
		t.Fatalf("second message: got object %d opcode %d size %d", msg.object, msg.opcode, size)
	}
	value, _, err := takeString(msg.body, 0)
	if err != nil {
		t.Fatal(err)
	}
	if value != "browser.work" {
		t.Fatalf("instance id round-trip: got %q", value)
	}
}

func TestNextMessageRejectsMalformedSize(t *testing.T) {
	// A declared size of 6: smaller than the header, so no progress could ever be made and
	// treating it as "read more" would spin forever.
	buf := []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x06, 0x00}
	if _, _, err := nextMessage(buf); err == nil {
		t.Fatal("a size smaller than the header must be an error, not a request for more bytes")
	}
	// A size that is not a multiple of four cannot be a real message either.
	buf = []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, 0x00}
	if _, _, err := nextMessage(buf); err == nil {
		t.Fatal("a size that is not word-aligned must be an error")
	}
}

func TestDecodeGlobal(t *testing.T) {
	body := appendUint32(nil, 9)
	body = appendString(body, managerInterface)
	body = appendUint32(body, 1)

	name, iface, version, err := decodeGlobal(body)
	if err != nil {
		t.Fatal(err)
	}
	if name != 9 || iface != managerInterface || version != 1 {
		t.Fatalf("got name=%d iface=%q version=%d", name, iface, version)
	}
}

func TestDecodeGlobalRejectsTruncated(t *testing.T) {
	body := appendUint32(nil, 9)
	body = appendString(body, managerInterface)
	// version missing
	if _, _, _, err := decodeGlobal(body); err == nil {
		t.Fatal("a global with no version must not decode")
	}
	// A length that claims more bytes than the message holds must be refused rather than
	// used to slice, since it is a number the compositor sent.
	lying := []byte{9, 0, 0, 0, 0xff, 0, 0, 0, 'a', 0, 0, 0}
	if _, _, _, err := decodeGlobal(lying); err == nil {
		t.Fatal("a string length running past the message must not decode")
	}
}

// wl_display.error is the compositor's whole vocabulary for "you got it wrong", and it
// arrives instead of the reply that was expected. Decoding it is the difference between a
// diagnosis and a timeout.
func TestDecodeDisplayError(t *testing.T) {
	body := appendUint32(nil, 5) // the offending object
	body = appendUint32(body, 2) // already_set
	body = appendString(body, "app_id has already been set")

	err := decodeDisplayError(body)
	if err == nil {
		t.Fatal("want an error")
	}
	var perr protocolError
	if !errors.As(err, &perr) {
		t.Fatalf("want a protocolError, got %T", err)
	}
	if perr.object != 5 || perr.code != 2 || perr.message != "app_id has already been set" {
		t.Fatalf("decoded %+v", perr)
	}
	for _, want := range []string{"object 5", "code 2", "app_id has already been set"} {
		if !bytes.Contains([]byte(err.Error()), []byte(want)) {
			t.Fatalf("error text %q does not mention %q", err.Error(), want)
		}
	}
}

// A body too short to decode still has to produce an error: the event itself is the news, and
// returning nil would let the launch believe the commit succeeded.
func TestDecodeDisplayErrorOnGarbage(t *testing.T) {
	if err := decodeDisplayError([]byte{1, 0, 0, 0}); err == nil {
		t.Fatal("an undecodable protocol error must still be an error")
	}
	truncated := append(appendUint32(nil, 5), appendUint32(nil, 2)...)
	err := decodeDisplayError(truncated) // no message string at all
	if err == nil {
		t.Fatal("a protocol error with no message must still be an error")
	}
}

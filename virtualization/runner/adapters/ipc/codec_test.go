package ipc

import (
	"strings"
	"testing"
)

func TestPrivateIPCBoundsAndStrictShape(check *testing.T) {
	for _, body := range []string{`{"Unknown":true}`, `{"PID":1} {}`, strings.Repeat(" ", Maximum+1), `null {}`} {
		var message struct{ PID int }
		if Decode(strings.NewReader(body), &message) == nil {
			check.Fatalf("accepted malformed helper message")
		}
	}
	var message struct{ PID int }
	if err := Decode(strings.NewReader(`{"PID":123}`), &message); err != nil || message.PID != 123 {
		check.Fatal(message, err)
	}
}

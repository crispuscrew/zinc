package ipc

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDecodeBoundedStrictMessage(t *testing.T) {
	for _, body := range []string{"", "{} {}", `{"Unknown":true}`, "{", strings.Repeat(" ", Maximum+1)} {
		var target struct{ Ready bool }
		if err := Decode(strings.NewReader(body), &target); err == nil {
			t.Errorf("accepted invalid message (%d bytes)", len(body))
		}
	}
	var target struct{ Ready bool }
	if err := Decode(strings.NewReader(`{"Ready":true}`), &target); err != nil || !target.Ready {
		t.Fatalf("valid message: %+v %v", target, err)
	}
}

func TestReceiveSilenceAndTimeout(t *testing.T) {
	for _, silence := range []bool{true, false} {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		defer writer.Close()
		if silence {
			writer.Close()
		}
		var target struct{ Ready bool }
		err = Receive(reader, &target, 20*time.Millisecond)
		if err == nil {
			t.Fatal("unacknowledged helper accepted")
		}
		if !silence && !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("unbounded readiness: %v", err)
		}
	}
}

func TestSendRejectsOversizedRequestBeforeWriting(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if err := Send(writer, strings.Repeat("x", Maximum)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatal(err)
	}
}

package ipc

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestReceiveWithinKeepsAnAbsoluteDeadline(check *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		check.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	watchdog := time.AfterFunc(2*time.Second, func() { reader.Close() })
	defer watchdog.Stop()
	if _, err := writer.Write([]byte(`{"PID":`)); err != nil {
		check.Fatal(err)
	}
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-finished:
				return
			case <-ticker.C:
				if _, err := writer.Write([]byte(" ")); err != nil {
					return
				}
			}
		}
	}()
	var message struct{ PID int }
	if err := ReceiveWithin(reader, &message, 40*time.Millisecond); !errors.Is(err, os.ErrDeadlineExceeded) {
		check.Fatalf("partial input escaped the receive budget: %v", err)
	}
}

func TestReceiveWithinRequiresPositiveBudget(check *testing.T) {
	for _, budget := range []time.Duration{0, -time.Millisecond} {
		if err := ReceiveWithin(nil, nil, budget); err == nil {
			check.Fatal("unbounded receive accepted")
		}
	}
}

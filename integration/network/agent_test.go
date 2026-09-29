package network_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const probeTimeout = 700 * time.Millisecond
const payload = "zinc-live-packet-echo"

type probeResult struct {
	Success  bool   `json:"success"`
	Timeout  bool   `json:"timeout"`
	Rejected bool   `json:"rejected"`
	Stage    string `json:"stage"`
	Error    string `json:"error,omitempty"`
}

func agent(arguments []string) int {
	if len(arguments) < 4 {
		fmt.Fprintln(os.Stderr, "invalid agent arguments")
		return 2
	}
	operation, protocol, address, portText := arguments[0], arguments[1], arguments[2], arguments[3]
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 2
	}
	if operation == "serve" {
		if protocol == "sctp" {
			err = serveSCTP(address, port)
		} else {
			err = serve(protocol, net.JoinHostPort(address, portText))
		}
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if operation != "probe" || len(arguments) != 6 {
		return 2
	}
	source, err := strconv.Atoi(arguments[5])
	if err != nil {
		return 2
	}
	var result probeResult
	if protocol == "sctp" {
		result = probeSCTP(arguments[4], source, address, port)
	} else {
		result = probe(protocol, arguments[4], source, address, port)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return 2
	}
	return 0
}

func serve(protocol, address string) error {
	if protocol == "tcp" {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return err
		}
		defer listener.Close()
		fmt.Println("READY")
		for {
			connection, err := listener.Accept()
			if err != nil {
				return err
			}
			go func() {
				defer connection.Close()
				_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
				_, _ = io.Copy(connection, connection)
			}()
		}
	}
	listener, err := net.ListenPacket("udp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	fmt.Println("READY")
	buffer := make([]byte, 512)
	for {
		length, remote, err := listener.ReadFrom(buffer)
		if err != nil {
			return err
		}
		if _, err := listener.WriteTo(buffer[:length], remote); err != nil {
			return err
		}
	}
}

func probe(protocol, source string, sourcePort int, destination string, port int) probeResult {
	dialer := net.Dialer{Timeout: probeTimeout}
	if protocol == "tcp" {
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(source), Port: sourcePort}
	} else {
		dialer.LocalAddr = &net.UDPAddr{IP: net.ParseIP(source), Port: sourcePort}
	}
	connection, err := dialer.Dial(protocol, net.JoinHostPort(destination, strconv.Itoa(port)))
	if err != nil {
		return failedProbe("connect", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(probeTimeout)); err != nil {
		return failedProbe("deadline", err)
	}
	if _, err := io.WriteString(connection, payload); err != nil {
		return failedProbe("write", err)
	}
	buffer := make([]byte, len(payload))
	if _, err := io.ReadFull(connection, buffer); err != nil {
		return failedProbe("read", err)
	}
	if string(buffer) != payload {
		return failedProbe("payload", fmt.Errorf("echo mismatch"))
	}
	return probeResult{Success: true, Stage: "echo"}
}

func failedProbe(stage string, err error) probeResult {
	var networkError net.Error
	timedOut := errors.As(err, &networkError) && networkError.Timeout()
	rejected := (stage == "write" || stage == "connect") && (errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES))
	return probeResult{Stage: stage, Error: strings.TrimSpace(err.Error()), Timeout: timedOut, Rejected: rejected}
}

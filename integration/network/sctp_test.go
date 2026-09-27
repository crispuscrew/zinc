package network_test

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

func socketAddress(address string, port int) (unix.Sockaddr, int) {
	parsed := net.ParseIP(address)
	if four := parsed.To4(); four != nil {
		result := &unix.SockaddrInet4{Port: port}
		copy(result.Addr[:], four)
		return result, unix.AF_INET
	}
	result := &unix.SockaddrInet6{Port: port}
	copy(result.Addr[:], parsed.To16())
	return result, unix.AF_INET6
}

func serveSCTP(address string, port int) error {
	endpoint, family := socketAddress(address, port)
	socket, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.IPPROTO_SCTP)
	if err != nil {
		return err
	}
	defer unix.Close(socket)
	if err := unix.Bind(socket, endpoint); err != nil {
		return err
	}
	if err := unix.Listen(socket, 16); err != nil {
		return err
	}
	fmt.Println("READY")
	for {
		connection, _, err := unix.Accept4(socket, unix.SOCK_CLOEXEC)
		if err != nil {
			return err
		}
		go func() {
			defer unix.Close(connection)
			buffer := make([]byte, 512)
			length, err := unix.Read(connection, buffer)
			if err == nil {
				_, _ = unix.Write(connection, buffer[:length])
			}
		}()
	}
}

func probeSCTP(source string, sourcePort int, destination string, port int) probeResult {
	local, family := socketAddress(source, sourcePort)
	remote, _ := socketAddress(destination, port)
	socket, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.IPPROTO_SCTP)
	if err != nil {
		return failedProbe("socket", err)
	}
	defer unix.Close(socket)
	if err := unix.Bind(socket, local); err != nil {
		return failedProbe("bind", err)
	}
	if err := unix.Connect(socket, remote); err != nil && err != unix.EINPROGRESS {
		return failedProbe("connect", err)
	}
	if result := pollSocket(socket, unix.POLLOUT); !result.Success {
		return result
	}
	status, err := unix.GetsockoptInt(socket, unix.SOL_SOCKET, unix.SO_ERROR)
	if err != nil {
		return failedProbe("status", err)
	}
	if status != 0 {
		return failedProbe("connect", unix.Errno(status))
	}
	if _, err := unix.Write(socket, []byte(payload)); err != nil {
		return failedProbe("write", err)
	}
	if result := pollSocket(socket, unix.POLLIN); !result.Success {
		return result
	}
	buffer := make([]byte, 512)
	length, err := unix.Read(socket, buffer)
	if err != nil {
		return failedProbe("read", err)
	}
	if string(buffer[:length]) != payload {
		return failedProbe("payload", fmt.Errorf("SCTP echo mismatch"))
	}
	return probeResult{Success: true, Stage: "echo"}
}

func pollSocket(socket int, events int16) probeResult {
	descriptors := []unix.PollFd{{Fd: int32(socket), Events: events}}
	count, err := unix.Poll(descriptors, int(probeTimeout/time.Millisecond))
	if err != nil {
		return failedProbe("poll", err)
	}
	if count == 0 {
		return probeResult{Stage: "poll", Timeout: true, Error: "SCTP poll timed out"}
	}
	return probeResult{Success: true}
}

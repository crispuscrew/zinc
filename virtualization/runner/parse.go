package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func parseForward(spec string) (vmoptions.PortForward, error) {
	var forward vmoptions.PortForward
	address, iface, _ := strings.Cut(spec, "@")
	ports, protocol, _ := strings.Cut(address, "/")
	forward.Interface, forward.Protocol = iface, protocol
	separator := strings.LastIndexByte(ports, ':')
	if separator < 0 {
		return forward, fmt.Errorf("--forward %q: require [BIND:]HOST:GUEST[/TCP|UDP][@NIC]", spec)
	}
	host, guest := ports[:separator], ports[separator+1:]
	if strings.Contains(host, ":") {
		bind, port, err := net.SplitHostPort(host)
		if err != nil {
			return forward, fmt.Errorf("--forward: %w", err)
		}
		forward.BindAddress, host = bind, port
	}
	var err error
	forward.HostPort, err = strconv.Atoi(host)
	if err != nil {
		return forward, fmt.Errorf("--forward: invalid host port")
	}
	forward.GuestPort, err = strconv.Atoi(guest)
	if err != nil {
		return forward, fmt.Errorf("--forward: invalid guest port")
	}
	return forward, nil
}

func parseResolution(spec string) (int, int, error) {
	if spec == "" {
		return 0, 0, nil
	}
	widthText, heightText, valid := strings.Cut(strings.ToLower(spec), "x")
	width, widthErr := strconv.Atoi(widthText)
	height, heightErr := strconv.Atoi(heightText)
	if !valid || widthErr != nil || heightErr != nil || width < 640 || height < 480 || width%2 != 0 {
		return 0, 0, fmt.Errorf("--resolution: require WxH, even width, at least 640x480")
	}
	if _, valid := schema.GuestDisplay(width, height); !valid {
		return 0, 0, fmt.Errorf("--resolution exceeds EDID limits")
	}
	return width, height, nil
}

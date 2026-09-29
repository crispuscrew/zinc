package netns

import (
	"fmt"
	"strconv"
	"strings"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func isolatedCommand(argv []string) ([]string, string, error) {
	if len(argv) == 0 {
		return nil, "", fmt.Errorf("empty QEMU command")
	}
	for index, argument := range argv {
		option := optionName(argument)
		if option == "netdev" || option == "nic" || option == "net" {
			return nil, "", fmt.Errorf("empty Interfaces requires no NIC or network backend")
		}
		if option == "device" && index+1 < len(argv) && networkDevice(argv[index+1]) {
			return nil, "", fmt.Errorf("empty Interfaces forbids network devices")
		}
	}
	return append([]string{}, argv...), "", nil
}

func checkAttachments(cfg schema.AppConfig, argv []string, manifest provision.Manifest) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty QEMU command")
	}
	wanted := map[string]provisionAttachment{}
	for index, iface := range cfg.NetworkMeta.Interfaces {
		for _, attachment := range manifest.Topology.Interfaces {
			if attachment.InterfaceID == iface.ID {
				wanted["net"+strconv.Itoa(index)] = provisionAttachment{attachment.Device, attachment.MAC}
			}
		}
	}
	backends, cards := map[string]bool{}, map[string]bool{}
	for index := 1; index < len(argv); index++ {
		argument := argv[index]
		option := optionName(argument)
		if option == "nic" || option == "net" || option == "netdev" && argument != "-netdev" || option == "device" && argument != "-device" {
			return fmt.Errorf("legacy QEMU network options bypass declared interfaces")
		}
		if argument != "-netdev" && argument != "-device" {
			continue
		}
		index++
		if index >= len(argv) {
			return fmt.Errorf("missing %s value", argument)
		}
		parts := strings.Split(argv[index], ",")
		properties := map[string]string{}
		for _, part := range parts[1:] {
			key, value, found := strings.Cut(part, "=")
			if !found {
				return fmt.Errorf("invalid QEMU property %q", part)
			}
			if _, exists := properties[key]; exists {
				return fmt.Errorf("duplicate QEMU property %q", key)
			}
			properties[key] = value
		}
		if argument == "-netdev" {
			if parts[0] != "tap" {
				return fmt.Errorf("full network policy requires provisioned TAPs; populate Service.NetworkAttachments before qemu.Args (slirp/hostfwd is not a packet-preserving uplink)")
			}
			attachment, exists := wanted[properties["id"]]
			if !exists || backends[properties["id"]] || properties["ifname"] != attachment.device || properties["script"] != "no" || properties["downscript"] != "no" || len(properties) != 4 {
				return fmt.Errorf("QEMU TAP differs from authoritative manifest")
			}
			backends[properties["id"]] = true
		} else if identifier := properties["netdev"]; identifier != "" {
			attachment, exists := wanted[identifier]
			if !exists || cards[identifier] || !strings.EqualFold(properties["mac"], attachment.mac) {
				return fmt.Errorf("QEMU NIC identity differs from provisioned MAC; use manifest MAC in runtime config")
			}
			cards[identifier] = true
		} else if networkDevice(argv[index]) {
			return fmt.Errorf("NIC requires an explicit provisioned netdev")
		}
	}
	if len(backends) != len(wanted) || len(cards) != len(wanted) {
		return fmt.Errorf("QEMU must attach exactly every declared NIC/TAP")
	}
	return nil
}

type provisionAttachment struct{ device, mac string }

func optionName(argument string) string {
	if !strings.HasPrefix(argument, "-") {
		return ""
	}
	name, _, _ := strings.Cut(strings.TrimLeft(argument, "-"), "=")
	return name
}

func networkDevice(value string) bool {
	name, _, _ := strings.Cut(value, ",")
	return strings.Contains(value, "netdev=") || strings.HasPrefix(name, "virtio-net") || strings.HasPrefix(name, "e1000") ||
		name == "rtl8139" || name == "vmxnet3" || name == "pcnet" || strings.HasPrefix(name, "ne2k") || name == "usb-net"
}

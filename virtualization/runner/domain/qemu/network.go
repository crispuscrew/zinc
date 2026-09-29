package qemu

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func networkArgs(cfg schema.AppConfig, layout Layout, identity string) []string {
	attachments := map[string]string{}
	for _, attachment := range layout.NetworkAttachments {
		attachments[attachment.InterfaceID] = attachment.TapName
	}
	var args []string
	for index, iface := range cfg.NetworkMeta.Interfaces {
		identifier := "net" + strconv.Itoa(index)
		backend := "user,id=" + identifier
		if tap := attachments[iface.ID]; tap != "" {
			backend = "tap,id=" + identifier + ",ifname=" + tap + ",script=no,downscript=no"
		} else {
			// A separate subnet for each slirp instance avoids overlapping guest routes.
			backend += fmt.Sprintf(",net=10.0.%d.0/24", index+2)
			if len(cfg.NetworkMeta.RulesByPriority) == 0 {
				backend += ",restrict=on"
			}
			for _, forward := range layout.Runtime.ForwardPorts {
				if forward.Interface != iface.ID && !(forward.Interface == "" && index == 0) {
					continue
				}
				bind := forward.Bind()
				// Preserve the requested host binding for the network adapter. Its
				// wrapper must publish that address, then rewrite the inner bind.
				if strings.Contains(bind, ":") {
					bind = "[" + bind + "]"
				}
				backend += fmt.Sprintf(",hostfwd=%s:%s:%d-:%d", strings.ToLower(forward.Transport()), bind, forward.HostPort, forward.GuestPort)
			}
		}
		device := "virtio-net-pci"
		if layout.Runtime.Devices == vmoptions.DevicesCompatible {
			device = "e1000e"
		}
		args = append(args, "-netdev", backend, "-device", device+",netdev="+identifier+",mac="+
			macFor(identity+"/"+iface.ID, iface.MacAddress, cfg.MinimizeFingerprint))
	}
	return args
}

func validateNetwork(cfg schema.AppConfig, layout Layout) error {
	interfaces := map[string]bool{}
	if len(cfg.NetworkMeta.Interfaces) > 254 {
		return fmt.Errorf("at most 254 logical NICs are supported")
	}
	for _, iface := range cfg.NetworkMeta.Interfaces {
		if err := vmoptions.Name(iface.ID); err != nil || interfaces[iface.ID] {
			return fmt.Errorf("invalid or duplicate network interface ID %q", iface.ID)
		}
		interfaces[iface.ID] = true
		if iface.MacAddress != "" {
			address, err := net.ParseMAC(iface.MacAddress)
			if err != nil || len(address) != 6 || address[0]&1 != 0 || iface.MacAddress == "00:00:00:00:00:00" {
				return fmt.Errorf("interface %s: require a nonzero unicast Ethernet MAC", iface.ID)
			}
		}
	}
	taps, attached := map[string]bool{}, map[string]bool{}
	for _, attachment := range layout.NetworkAttachments {
		if !interfaces[attachment.InterfaceID] || attached[attachment.InterfaceID] || taps[attachment.TapName] {
			return fmt.Errorf("invalid or duplicate TAP attachment for %q", attachment.InterfaceID)
		}
		if err := vmoptions.Name(attachment.TapName); err != nil || len(attachment.TapName) > 15 {
			return fmt.Errorf("invalid TAP device name %q", attachment.TapName)
		}
		attached[attachment.InterfaceID], taps[attachment.TapName] = true, true
	}
	if len(attached) > 0 && len(attached) != len(interfaces) {
		return fmt.Errorf("TAP attachments must cover every declared logical NIC")
	}
	for _, forward := range layout.Runtime.ForwardPorts {
		if len(interfaces) == 0 || (forward.Interface != "" && !interfaces[forward.Interface]) {
			return fmt.Errorf("port forward names an absent logical NIC %q", forward.Interface)
		}
	}
	if len(attached) > 0 {
		return nil // the provisioner owns packet transport and publication for TAPs
	}
	for _, rule := range cfg.NetworkMeta.RulesByPriority {
		for _, protocol := range rule.Protocols {
			switch protocol {
			case schema.NetworkTCP, schema.NetworkUDP, schema.NetworkICMP, schema.NetworkICMPv6:
			default:
				return fmt.Errorf("protocol %s requires a provisioned TAP transport; slirp cannot carry it", protocol)
			}
		}
		if rule.From.Interface != "" || rule.To.Interface != "" {
			return fmt.Errorf("interface-scoped policy requires provisioned TAPs; slirp merges guest sockets")
		}
	}
	return nil
}

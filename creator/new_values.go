package main

import (
	"bytes"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

func decodeFlag(target any) func(string) error {
	return func(value string) error {
		decoder := yaml.NewDecoder(bytes.NewBufferString(value))
		decoder.KnownFields(true)
		if err := decoder.Decode(target); err != nil {
			return err
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return fmt.Errorf("expected one YAML value, got additional data: %v", err)
		}
		return nil
	}
}

func appendArg(target *[]string) func(string) error {
	return func(value string) error { *target = append(*target, value); return nil }
}

func envFlag(target *map[string]string) func(string) error {
	return func(value string) error {
		name, content, found := strings.Cut(value, "=")
		if !found || name == "" {
			return fmt.Errorf("environment entry must be NAME=VALUE")
		}
		if *target == nil {
			*target = map[string]string{}
		}
		if _, exists := (*target)[name]; exists {
			return fmt.Errorf("duplicate environment variable %q", name)
		}
		(*target)[name] = content
		return nil
	}
}

func registerComplex(flags *flag.FlagSet, cfg *schema.AppConfig) {
	flags.Func("network", "NetworkMeta as YAML/JSON; empty Interfaces means no NIC", decodeFlag(&cfg.NetworkMeta))
	flags.Func("interface", "NIC ID[=MAC]; repeatable", func(value string) error {
		name, address, _ := strings.Cut(value, "=")
		cfg.NetworkMeta.Interfaces = append(cfg.NetworkMeta.Interfaces, schema.NetworkInterface{ID: name, MacAddress: address})
		return nil
	})
	flags.Func("network-rule", "one ordered NetworkRule as YAML/JSON; repeatable", func(value string) error {
		var rule schema.NetworkRule
		if err := decodeFlag(&rule)(value); err != nil {
			return err
		}
		cfg.NetworkMeta.RulesByPriority = append(cfg.NetworkMeta.RulesByPriority, rule)
		return nil
	})
	flags.Func("dns-resolver", "one prioritized DNSResolver as YAML/JSON; repeatable", func(value string) error {
		var resolver schema.DNSResolver
		if err := decodeFlag(&resolver)(value); err != nil {
			return err
		}
		cfg.NetworkMeta.DNS.ResolversByPriority = append(cfg.NetworkMeta.DNS.ResolversByPriority, resolver)
		return nil
	})
	flags.Func("volumes", "Volume array as YAML/JSON", decodeFlag(&cfg.Volumes))
	flags.Func("configs", "ConfigFile array as YAML/JSON", decodeFlag(&cfg.Configs))
	flags.Func("keys", "Key array as YAML/JSON", decodeFlag(&cfg.Keys))
	flags.Func("notifications", "NotificationMeta as YAML/JSON", decodeFlag(&cfg.NotificationMeta))
	for _, direction := range []struct {
		name   string
		device *schema.AudioDevice
	}{
		{"playback", &cfg.AudioMeta.Playback}, {"microphone", &cfg.AudioMeta.Microphone}, {"monitor", &cfg.AudioMeta.Monitor},
	} {
		flags.BoolVar(&direction.device.PipeWireDefault, direction.name+"-default", false, "grant the default PipeWire device for this direction")
		flags.Func(direction.name+"-pipewire", "exact PipeWire device name; repeatable", appendArg(&direction.device.PipeWireDevices))
		flags.Func(direction.name+"-alsa", "exact ALSA device node; repeatable", appendArg(&direction.device.ALSADevices))
	}
}

func splitList(spec string) []string {
	var entries []string
	for _, entry := range strings.Split(spec, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseResolution(spec string) (int, int, error) {
	if strings.TrimSpace(spec) == "" {
		return 0, 0, nil
	}
	parts := strings.Split(strings.ToLower(strings.TrimSpace(spec)), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("--resolution %q: want WxH", spec)
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("--resolution width %q is not a number", parts[0])
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("--resolution height %q is not a number", parts[1])
	}
	return width, height, nil
}

func resolveMac(value string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(value), "random") {
		return value, nil
	}
	var octets [6]byte
	if _, err := rand.Read(octets[:]); err != nil {
		return "", fmt.Errorf("generate MAC: %w", err)
	}
	octets[0] = (octets[0] | 0x02) &^ 0x01
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", octets[0], octets[1], octets[2], octets[3], octets[4], octets[5]), nil
}

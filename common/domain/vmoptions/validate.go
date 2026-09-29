package vmoptions

import (
	"errors"
	"fmt"
	"math"
	"net/netip"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Name is also used before constructing an options-store path.
func Name(name string) error {
	if len(name) > 96 || !namePattern.MatchString(name) {
		return fmt.Errorf("AppNameID %q: require 1-96 lowercase alphanumeric, '.', '_' or '-' characters, starting alphanumeric", name)
	}
	return nil
}

// Path rejects QEMU property delimiters even when exec preserves argv boundaries.
func Path(path string) error {
	if path == "/" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("%q: require a clean absolute path", path)
	}
	if strings.ContainsAny(path, ",:") || strings.ContainsFunc(path, unicode.IsControl) {
		return fmt.Errorf("%q: QEMU paths must not contain commas, colons or control characters", path)
	}
	return nil
}

// Validate is pure; checking files, app binding, and host capabilities is separate.
func Validate(config Config) error {
	var problems []error
	add := func(format string, values ...any) { problems = append(problems, fmt.Errorf(format, values...)) }
	if config.Version != Version {
		add("Version: got %d, want %d", config.Version, Version)
	}
	if err := Name(config.AppNameID); err != nil {
		problems = append(problems, err)
	}
	if err := Path(config.Image); err != nil {
		add("Image: %v", err)
	}
	if !digestPattern.MatchString(config.BaseDigest) {
		add("BaseDigest: require sha256:<64 lowercase hexadecimal characters>")
	}
	if config.DiskSizeGiB < 0 || config.DiskSizeGiB > math.MaxInt64/(1<<30) {
		add("DiskSizeGiB: outside the supported byte-size range")
	}
	switch config.Display {
	case "", DisplayNone, DisplayWindow, DisplayAccelerated, DisplayCompatible:
	default:
		add("Display %q: unknown display profile", config.Display)
	}
	switch config.Devices {
	case "", DevicesVirtio, DevicesCompatible:
	default:
		add("Devices %q: unknown device profile", config.Devices)
	}
	if len(config.InstallMedia) > 6 {
		add("InstallMedia: at most six discs fit the dedicated AHCI controller")
	}
	seenMedia := map[string]bool{}
	for index, media := range config.InstallMedia {
		if err := Path(media); err != nil {
			add("InstallMedia[%d]: %v", index, err)
		}
		if seenMedia[media] || media == config.Image {
			add("InstallMedia[%d]: duplicate medium or base image", index)
		}
		seenMedia[media] = true
	}
	for index, forward := range config.ForwardPorts {
		if forward.Transport() != "TCP" && forward.Transport() != "UDP" {
			add("ForwardPorts[%d].Protocol: require TCP or UDP", index)
		}
		address, err := netip.ParseAddr(forward.Bind())
		if err != nil || address.Zone() != "" || address.IsMulticast() {
			add("ForwardPorts[%d].BindAddress: require a unicast or wildcard IP literal without a zone", index)
		}
		if forward.HostPort < 1024 || forward.HostPort > 65535 || forward.GuestPort < 1 || forward.GuestPort > 65535 {
			add("ForwardPorts[%d]: host port must be 1024-65535; guest port must be 1-65535", index)
		}
		if forward.Interface != "" && !namePattern.MatchString(forward.Interface) {
			add("ForwardPorts[%d].Interface: invalid logical interface ID", index)
		}
		for _, prior := range config.ForwardPorts[:index] {
			other, otherErr := netip.ParseAddr(prior.Bind())
			if err == nil && otherErr == nil && forward.Transport() == prior.Transport() && forward.HostPort == prior.HostPort &&
				(address.Unmap() == other.Unmap() || address.IsUnspecified() || other.IsUnspecified()) {
				add("ForwardPorts[%d]: conflicting host binding", index)
			}
		}
	}
	return errors.Join(problems...)
}

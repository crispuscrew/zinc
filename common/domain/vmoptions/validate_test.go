package vmoptions

import (
	"strings"
	"testing"
)

func validConfig() Config {
	config := Default("guest", "/images/base.qcow2")
	config.BaseDigest = "sha256:" + strings.Repeat("a", 64)
	return config
}

func TestDefaultRequiresExplicitPin(t *testing.T) {
	config := Default("guest", "/images/base.qcow2")
	if err := Validate(config); err == nil || !strings.Contains(err.Error(), "BaseDigest") {
		t.Fatalf("got %v", err)
	}
	config = validConfig()
	if err := Validate(config); err != nil {
		t.Fatal(err)
	}
	if config.Display != "" || config.DiskSizeGiB != 0 {
		t.Fatal("defaults must defer display and preserve disk size")
	}
}

func TestInvalidRuntimeValues(t *testing.T) {
	cases := map[string]func(*Config){
		"version":            func(config *Config) { config.Version = 2 },
		"name":               func(config *Config) { config.AppNameID = "../other" },
		"relative":           func(config *Config) { config.Image = "base.qcow2" },
		"property injection": func(config *Config) { config.Image = "/images/x,file=/elsewhere" },
		"size overflow":      func(config *Config) { config.DiskSizeGiB = 1 << 62 },
		"display":            func(config *Config) { config.Display = "Typo" },
		"devices":            func(config *Config) { config.Devices = "Typo" },
		"duplicate media":    func(config *Config) { config.InstallMedia = []string{"/iso/one.iso", "/iso/one.iso"} },
		"protocol": func(config *Config) {
			config.ForwardPorts = []PortForward{{Protocol: "SCTP", HostPort: 2222, GuestPort: 22}}
		},
		"privileged": func(config *Config) { config.ForwardPorts = []PortForward{{HostPort: 22, GuestPort: 22}} },
		"binding": func(config *Config) {
			config.ForwardPorts = []PortForward{{HostPort: 2222, GuestPort: 22, BindAddress: "localhost"}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			config := validConfig()
			mutate(&config)
			if Validate(config) == nil {
				t.Fatal("invalid runtime options accepted")
			}
		})
	}
}

func TestPortBindingCollisions(t *testing.T) {
	config := validConfig()
	config.ForwardPorts = []PortForward{{HostPort: 2222, GuestPort: 22}, {Protocol: "UDP", HostPort: 2222, GuestPort: 53}}
	if err := Validate(config); err != nil {
		t.Fatal(err)
	}
	config.ForwardPorts[1].Protocol = "TCP"
	config.ForwardPorts[1].BindAddress = "0.0.0.0"
	if Validate(config) == nil {
		t.Fatal("wildcard collision accepted")
	}
}

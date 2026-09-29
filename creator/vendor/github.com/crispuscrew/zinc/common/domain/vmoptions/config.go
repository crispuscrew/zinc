// Package vmoptions describes host-local VM choices, separately from app YAML.
package vmoptions

const Version = 1

type Display string

const (
	DisplayNone        Display = "None"
	DisplayWindow      Display = "Window"
	DisplayAccelerated Display = "Accelerated"
	DisplayCompatible  Display = "Compatible"
)

type Devices string

const (
	DevicesVirtio     Devices = "Virtio"
	DevicesCompatible Devices = "Compatible"
)

type Config struct {
	Version      int           `json:"Version"`
	AppNameID    string        `json:"AppNameID"`
	Image        string        `json:"Image"`
	BaseDigest   string        `json:"BaseDigest"`
	DiskSizeGiB  int64         `json:"DiskSizeGiB"`
	Display      Display       `json:"Display"`
	Devices      Devices       `json:"Devices"`
	InstallMedia []string      `json:"InstallMedia"`
	ForwardPorts []PortForward `json:"ForwardPorts"`
}

type PortForward struct {
	Protocol    string `json:"Protocol"`
	BindAddress string `json:"BindAddress"`
	HostPort    int    `json:"HostPort"`
	GuestPort   int    `json:"GuestPort"`
	Interface   string `json:"Interface"`
}

// Default leaves the pin unset: calculating a digest must never authorize it.
// Empty Display is resolved against the app's Terminal and DisableGpuAccess fields.
func Default(name, image string) Config {
	return Config{Version: Version, AppNameID: name, Image: image, Devices: DevicesVirtio}
}

func (forward PortForward) Bind() string {
	if forward.BindAddress == "" {
		return "127.0.0.1"
	}
	return forward.BindAddress
}

func (forward PortForward) Transport() string {
	if forward.Protocol == "" {
		return "TCP"
	}
	return forward.Protocol
}

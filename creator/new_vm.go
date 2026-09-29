package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

type vmInputs struct {
	memory                                                      int64
	cpus                                                        int
	display, devices, firmware, forward, media, resolution, mac string
}

func registerVM(flags *flag.FlagSet, cfg *schema.AppConfig, opts *vmoptions.Config) *vmInputs {
	input := &vmInputs{}
	flags.StringVar(&opts.BaseDigest, "base-digest", "", "VM base disk sha256 pin")
	flags.Int64Var(&opts.DiskSizeGiB, "disk", 0, "VM overlay size in GiB")
	flags.Int64Var(&input.memory, "memory", 4096, "VM RAM in MiB (alias for --ram)")
	flags.IntVar(&input.cpus, "vcpus", 2, "VM whole CPU count (alias for --cpus)")
	flags.StringVar(&input.display, "display", string(opts.Display), "VM None|Window|Accelerated|Compatible")
	flags.StringVar(&input.devices, "devices", string(opts.Devices), "VM Virtio|Compatible")
	flags.StringVar(&input.firmware, "firmware", "UEFI", "VM BIOS|UEFI")
	flags.BoolVar(&cfg.StartConditions.LoaderBIOS, "loader-bios", false, "VM: BIOS instead of UEFI")
	flags.BoolVar(&cfg.StartConditions.SecureBoot, "secure-boot", false, "VM UEFI Secure Boot")
	flags.BoolVar(&cfg.StartConditions.TPM, "tpm", false, "VM TPM 2.0")
	flags.BoolVar(&cfg.ImageMeta.CloudInit, "cloud-init", true, "VM cloud-init provisioning")
	flags.StringVar(&cfg.ImageMeta.PublicSSHKeyPath, "ci-ssh-key", "", "VM public SSH key path")
	flags.Func("ci-user", "VM cloud-init user", func(value string) error {
		cfg.InternalUserMeta.NonRootUserName = value
		cfg.InternalUserMeta.UseNonRootUser = value != ""
		return nil
	})
	flags.BoolVar(&cfg.DisplayMeta.Vulkan, "vulkan", false, "VM Vulkan (backend may relax its sandbox)")
	flags.StringVar(&input.resolution, "resolution", "", "VM fixed display WxH")
	flags.StringVar(&input.mac, "mac", "", "VM primary NIC MAC, or random (creates a NIC, not a rule)")
	flags.StringVar(&input.media, "media", "", "VM read-only ISO paths, comma-separated")
	flags.StringVar(&input.forward, "forward", "", "VM loopback TCP HOST:GUEST forwards, comma-separated")
	return input
}

func (input *vmInputs) apply(flags *flag.FlagSet, cfg *schema.AppConfig, opts *vmoptions.Config) error {
	used := map[string]bool{}
	flags.Visit(func(value *flag.Flag) { used[value.Name] = true })
	if used["memory"] && used["ram"] || used["vcpus"] && used["cpus"] {
		return fmt.Errorf("use one spelling for each VM resource: --memory/--ram, --vcpus/--cpus")
	}
	if !used["ram"] {
		cfg.ResourcesMeta.MaxRamMiB = input.memory
	}
	if !used["cpus"] {
		cfg.ResourcesMeta.MaxCPUCores = float64(input.cpus)
	}
	if input.firmware != "BIOS" && input.firmware != "UEFI" {
		return fmt.Errorf("--firmware: want BIOS or UEFI")
	}
	if used["firmware"] && used["loader-bios"] && cfg.StartConditions.LoaderBIOS != (input.firmware == "BIOS") {
		return fmt.Errorf("--firmware conflicts with --loader-bios")
	}
	if used["firmware"] {
		cfg.StartConditions.LoaderBIOS = input.firmware == "BIOS"
	}
	opts.Display, opts.Devices = vmoptions.Display(input.display), vmoptions.Devices(input.devices)
	opts.InstallMedia = splitList(input.media)
	var err error
	opts.ForwardPorts, err = parseForwards(input.forward)
	if err != nil {
		return err
	}
	cfg.DisplayMeta.DisplayWidth, cfg.DisplayMeta.DisplayHeight, err = parseResolution(input.resolution)
	if err != nil {
		return err
	}
	if input.mac != "" {
		address, err := resolveMac(input.mac)
		if err != nil {
			return err
		}
		if len(cfg.NetworkMeta.Interfaces) != 0 {
			return fmt.Errorf("--mac conflicts with explicit interfaces; put the MAC in --interface ID=MAC")
		}
		cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary", MacAddress: address}}
	}
	return seedVMForwards(cfg, opts, used["network"])
}

// --forward explicitly requests host ingress. Materialize only that allowance,
// never internet egress or cross-app access.
func seedVMForwards(cfg *schema.AppConfig, opts *vmoptions.Config, explicitNetwork bool) error {
	if len(opts.ForwardPorts) == 0 {
		return nil
	}
	if len(cfg.NetworkMeta.Interfaces) == 0 {
		if explicitNetwork {
			return fmt.Errorf("--forward conflicts with explicitly empty NetworkMeta.Interfaces")
		}
		cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary"}}
	}
	if len(cfg.NetworkMeta.Interfaces) != 1 {
		return fmt.Errorf("--forward requires one NIC; configure per-interface forwards in VM runtime options for multiple NICs")
	}
	iface := cfg.NetworkMeta.Interfaces[0].ID
	for index := range opts.ForwardPorts {
		opts.ForwardPorts[index].Interface = iface
		cfg.NetworkMeta.RulesByPriority = append(cfg.NetworkMeta.RulesByPriority, schema.NetworkRule{
			From:      schema.NetworkPeer{Type: schema.NetworkPeerHost},
			To:        schema.NetworkPeer{Type: schema.NetworkPeerSelf, Interface: iface, Filter: schema.NetworkPeerFilter{Ports: []int{opts.ForwardPorts[index].GuestPort}}},
			Protocols: []schema.NetworkProtocol{schema.NetworkTCP},
		})
	}
	return nil
}

func vmFlagUsed(flags *flag.FlagSet) bool {
	used := false
	flags.Visit(func(value *flag.Flag) {
		switch value.Name {
		case "base-digest", "disk", "memory", "vcpus", "display", "devices", "firmware", "loader-bios", "secure-boot", "tpm", "cloud-init", "ci-user", "ci-ssh-key", "vulkan", "resolution", "mac", "media", "forward":
			used = true
		}
	})
	return used
}

func parseForwards(spec string) ([]vmoptions.PortForward, error) {
	var forwards []vmoptions.PortForward
	for _, pair := range splitList(spec) {
		hostText, guestText, found := strings.Cut(pair, ":")
		host, hostErr := strconv.Atoi(hostText)
		guest, guestErr := strconv.Atoi(guestText)
		if !found || hostErr != nil || guestErr != nil {
			return nil, fmt.Errorf("--forward %q: want HOST:GUEST numeric ports", pair)
		}
		forwards = append(forwards, vmoptions.PortForward{Protocol: "TCP", BindAddress: "127.0.0.1", HostPort: host, GuestPort: guest})
	}
	return forwards, nil
}

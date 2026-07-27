package schema

// SchemaVersion is the only app-config schema version this build understands.
const SchemaVersion = 2

type Type string

const (
	ZincContainer      Type = "ZincContainer"
	ZincVirtualization Type = "ZincVirtualization"
)

// AppConfig is one app definition: ~/.config/zinc/apps/<name>.yaml
// Most parameters can be overridden at app start
type AppConfig struct {
	SchemaVersion int  `yaml:"SchemaVersion"`
	Type          Type `yaml:"Type"` // VM vs Container, "" interpreted as error

	AppNameID   string `yaml:"AppNameID"` // Also using as container/vm name
	Icon        string `yaml:"Icon"`
	Description string `yaml:"Description"`
	Group       string `yaml:"Group"` // optional category, for grouping in a launcher; presentation-only

	StartConditions StartConditions `yaml:"StartConditions"`
	StopConditions  StopConditions  `yaml:"StopConditions"`

	ResourcesMeta    ResourcesMeta    `yaml:"ResourcesMeta"`
	InternalUserMeta InternalUserMeta `yaml:"InternalUserMeta"`
	ImageMeta        ImageMeta        `yaml:"ImageMeta"`
	DisplayMeta      DisplayMeta      `yaml:"DisplayMeta"`
	NetworkMeta      NetworkMeta      `yaml:"NetworkMeta"`
	NotificationMeta NotificationMeta `yaml:"NotificationMeta"`

	// VirtualizationMeta applies only to Type: ZincVirtualization. A container app must
	// leave it at its zero value; validation rejects it rather than ignoring it, so a
	// field can never look configured while doing nothing.
	VirtualizationMeta VirtualizationMeta `yaml:"VirtualizationMeta"`

	Configs      []Volume  `yaml:"Configs"` // Use host local path from app_name/configs/ folder
	Volumes      []Volume  `yaml:"Volumes"` // extra host bind mounts can also be added for one run via `zcr run -v` (not persisted here)
	Keys         []Key     `yaml:"Keys"`
	HostTheme    bool      `yaml:"HostTheme"`
	AudioMeta    AudioMeta `yaml:"AudioMeta"`
	Capabilities []string  `yaml:"Capabilities"` // if container --cap-add entries
}

type StartConditions struct {
	DependsOn []string `yaml:"DependsOn"` // apps, which must be running while/starting with it

	Autorestart bool `yaml:"Autorestart"` // Autorestart if falls, not restart if manually closed

	Entrypoint              string `yaml:"Entrypoint"`              // if empty use app default
	Terminal                bool   `yaml:"Terminal"`                // if true, create terminal window for it
	Multiterminal           bool   `yaml:"Multiterminal"`           // createable attached terminals, every terminal hold container to live
	MultiterminalEntrypoint string `yaml:"MultiterminalEntrypoint"` // if empty use Entrypoint
}

type StopConditions struct {
	KeepAlive  bool `yaml:"KeepAlive"`  // Stays freeze/alive after entrypoint finish
	Background bool `yaml:"Background"` // Stays alive after window close
}

type ResourcesMeta struct {
	MaxCPUCores float64 `yaml:"MaxCPUCores"` // Can be 0.5, for example
	MaxRamMiB   int64   `yaml:"MaxRamMiB"`
	MaxSwapMiB  int64   `yaml:"MaxSwapMiB"` // Only if swap accessible
	PIDsLimit   int64   `yaml:"PIDsLimit"`  // For fork-bomb prevented
}

type InternalUserMeta struct {
	UseNonRootUser  bool   `yaml:"UseNonRootUser"` // If true using NonRootUser
	KeepUserID      bool   `yaml:"KeepUserID"`     // Keep the same id and etc as real host user
	NonRootUserName string `yaml:"NonRootUserName"`
}

type ImageMeta struct {
	// Image is where the app comes from, read according to Type: a container reference for
	// ZincContainer (digest-pinned unless localhost/), or the path of a base disk image for
	// ZincVirtualization (pinned by VirtualizationMeta.BaseDigest, since a file's hash
	// cannot ride inside its path).
	Image string `yaml:"Image"`
	// Install is the app's setup steps, one per entry. For a container they become the
	// single RUN layer of the derived image; for a VM they become cloud-init runcmd lines
	// on first boot. Same intent either way: what to add on top of the pinned base.
	Install []string `yaml:"Install"`
}

// VirtualizationMeta configures a VM app: the guest's hardware, how you see it, and the
// first-boot identity handed to it. The base disk named by ImageMeta.Image is never
// written to - each app gets its own copy-on-write overlay - so deleting the overlay
// resets the app to the authored base, which is the VM analogue of a fresh container.
type VirtualizationMeta struct {
	// BaseDigest is the sha256 the base disk image must hash to, as "sha256:<64 hex>". A
	// VM base is a file rather than a registry reference, so the pin cannot ride inside
	// the reference the way a container digest does - but the rule is the same one: what
	// runs must be what was authorised.
	BaseDigest string `yaml:"BaseDigest"`

	DiskSizeGiB int64 `yaml:"DiskSizeGiB"` // virtual size of the app's overlay; 0 keeps the base image's size
	MemoryMiB   int64 `yaml:"MemoryMiB"`   // guest RAM, allocated not capped
	VCPUs       int   `yaml:"VCPUs"`       // guest CPUs

	Display VMDisplay `yaml:"Display"`

	// DisplayWidth and DisplayHeight fix the guest's screen size. Both zero leaves it to
	// the firmware, which settles on 1280x800 and stays there: a guest with no graphics
	// driver takes the resolution UEFI hands it at boot and cannot change it afterwards,
	// so resizing the window just scales those pixels. Setting them is what makes a
	// driverless guest, Windows above all, come up at a usable size.
	DisplayWidth  int `yaml:"DisplayWidth"`
	DisplayHeight int `yaml:"DisplayHeight"`

	// Vulkan passes the guest's Vulkan calls through to the host GPU (qemu's venus). It is
	// off by default and must be asked for, because it costs real things: qemu's own seccomp
	// sandbox has to be disabled (the venus renderer runs in a helper process that the
	// sandbox kills), and the host needs a virglrenderer built with venus support, which
	// distributions generally do not ship. Without it a guest still gets accelerated
	// OpenGL, but its Vulkan runs on the CPU - which is what Proton, DXVK and vkd3d use.
	Vulkan bool `yaml:"Vulkan"`

	// Firmware is how the guest boots. Linux images generally boot either way; Windows 11
	// requires UEFI, and refuses to install without it.
	Firmware VMFirmware `yaml:"Firmware"`
	// SecureBoot enables UEFI Secure Boot, which Windows 11 also expects. Requires UEFI.
	SecureBoot bool `yaml:"SecureBoot"`
	// TPM attaches an emulated TPM 2.0. Windows 11 refuses to install without one.
	TPM bool `yaml:"TPM"`

	// Devices picks which hardware the guest is given. It exists because a guest can only
	// use hardware it has drivers for, and Windows Setup has none for virtio: pointed at a
	// virtio disk it reports finding no drives at all.
	Devices VMDevices `yaml:"Devices"`

	// InstallMedia are ISO images attached read-only as CD-ROMs. A Windows guest is
	// installed from one rather than started from a cloud image; `zvr install` boots with
	// these attached, and a normal run ignores them.
	InstallMedia []string `yaml:"InstallMedia"`

	// ForwardPorts publishes a guest port on the host over user-mode networking. VM apps
	// do not use NetworkMeta: that model is enforced by nftables inside a container's own
	// network namespace and does not carry over to a guest, so rather than mis-enforce it
	// a VM app is limited to these explicit forwards.
	ForwardPorts []PortForward `yaml:"ForwardPorts"`

	// MacAddress overrides the guest NIC's address. Left empty, Zinc derives one from the
	// app name under QEMU's own 52:54:00 prefix, which is unique per app but says plainly
	// that the machine is a QEMU guest. Set this to present something else - a
	// locally-administered address (first octet 02, 06, 0a or 0e) belongs to no vendor and
	// so identifies nothing.
	MacAddress string `yaml:"MacAddress"`

	CloudInit CloudInit `yaml:"CloudInit"`
}

// IsZero reports whether nothing in the VM group was set. Validation uses it to catch VM
// fields left on a container app, where they would be inert.
func (virt VirtualizationMeta) IsZero() bool {
	return virt.BaseDigest == "" &&
		virt.DiskSizeGiB == 0 && virt.MemoryMiB == 0 && virt.VCPUs == 0 &&
		virt.Display == "" && virt.DisplayWidth == 0 && virt.DisplayHeight == 0 && !virt.Vulkan &&
		virt.Firmware == "" && !virt.SecureBoot && !virt.TPM &&
		virt.Devices == "" && len(virt.InstallMedia) == 0 &&
		len(virt.ForwardPorts) == 0 && virt.MacAddress == "" &&
		virt.CloudInit == CloudInit{}
}

// VMFirmware is how a guest boots.
type VMFirmware string

const (
	// VMFirmwareBIOS is the traditional path, and what a Linux cloud image expects.
	VMFirmwareBIOS VMFirmware = "BIOS"
	// VMFirmwareUEFI boots through OVMF, with its own writable variable store per app so a
	// guest's boot entries survive a restart without leaking into any other guest.
	VMFirmwareUEFI VMFirmware = "UEFI"
)

// VMDevices is the hardware profile a guest is given. Virtio is faster in every dimension
// and is what a Linux guest should use; Compatible exists because a guest cannot use
// hardware it has no driver for, and Windows Setup ships none for virtio - pointed at a
// virtio disk it simply reports that it cannot find a drive.
type VMDevices string

const (
	// VMDevicesVirtio is the default: virtio disk, network and input.
	VMDevicesVirtio VMDevices = "Virtio"
	// VMDevicesCompatible uses hardware every mainstream OS has drivers for out of the box:
	// an AHCI disk, an Intel gigabit NIC and a USB tablet. Slower, and the price of being
	// able to install an OS that has never heard of virtio.
	VMDevicesCompatible VMDevices = "Compatible"
)

// VMDisplay is how a VM app is seen. It is explicit rather than inferred: whether a guest
// gets an accelerated window is exactly the difference between a usable game and an
// unusable one, so it is not something to guess from other fields.
type VMDisplay string

const (
	// VMDisplayNone runs headless - no window, reachable over the serial console. The only
	// mode that works without a display, so it is what a test or a server-ish guest uses.
	VMDisplayNone VMDisplay = "None"
	// VMDisplayWindow opens a local window with no 3D acceleration. The fallback when a
	// guest or host lacks working virtio-gpu.
	VMDisplayWindow VMDisplay = "Window"
	// VMDisplayAccelerated opens a local window backed by virtio-gpu-gl, so guest 3D runs
	// on the host GPU and frames reach the compositor without leaving the machine. Needs a
	// guest with the virtio-gpu driver (Linux).
	VMDisplayAccelerated VMDisplay = "Accelerated"
	// VMDisplayCompatible opens a local window on plain VGA, which every OS can drive
	// including at install time. No acceleration of any kind: it is what a guest without a
	// virtio-gpu driver gets, and on this hardware that means Windows.
	VMDisplayCompatible VMDisplay = "Compatible"
)

// PortForward publishes GuestPort inside the VM as HostPort on the host.
type PortForward struct {
	HostPort  int `yaml:"HostPort"`
	GuestPort int `yaml:"GuestPort"`
}

// CloudInit is the first-boot identity written to a seed ISO and handed to the guest, so
// a freshly created VM is reachable without an interactive install.
type CloudInit struct {
	// Disabled skips the seed ISO entirely, for a base image that provisions itself or
	// one already configured.
	Disabled bool `yaml:"Disabled"`

	UserName string `yaml:"UserName"` // the account created in the guest; empty = the image's default
	// SSHKeyPath is a host path to a PUBLIC key authorised for UserName. Public, never
	// private: the seed ISO is readable by the guest, so a private key put here would be
	// handed to it.
	SSHKeyPath string `yaml:"SSHKeyPath"`
}

type DisplayMeta struct {
	DisableSecurityContext bool `yaml:"DisableSecurityContext"` // security-context | passthrough
	DisableGpuAccess       bool `yaml:"DisableGpuAccess"`
}

type NetworkMeta struct {
	// The first entry is priority
	NetworkLists []NetworkList `yaml:"NetworkLists"`
}

type NetworkList struct {
	Host      bool   `yaml:"Host"`      // it list for host or container?
	AppName   string `yaml:"AppName"`   // if host == false, which app net do we use, "" == this app
	Interface string `yaml:"Interface"` // if u want to reach concrete interface of app/host

	Blacklist bool `yaml:"Blacklist"` // or whitelist

	Ingress bool `yaml:"Ingress"` // false = egress rule (default); true = Ports are this app's own listeners, exposed to the scope

	IPv4CIDR []string `yaml:"IPv4CIDR"`
	IPv6CIDR []string `yaml:"IPv6CIDR"`
	Ports    []int    `yaml:"Ports"`

	GatewayV4 string `yaml:"GatewayV4"` // if "" use default
	GatewayV6 string `yaml:"GatewayV6"`
}

type NotificationMeta struct {
	Disabled bool `yaml:"Disabled"`
	Silenced bool `yaml:"Silenced"` // All notification from app will be silenced

	UseCustomPrefix bool   `yaml:"UseCustomPrefix"`
	CustomPrefix    string `yaml:"CustomPrefix"`

	AllowedActions   bool `yaml:"AllowedActions"`
	AllowedProlonged bool `yaml:"AllowedProlonged"` // Allowed expire_timeout > cfg.notifications.Prolonged
	AllowedLinks     bool `yaml:"AllowedLinks"`
}

// Readable drop, because u cannot mount something u cannot read
type Volume struct {
	InnerMount string `yaml:"InnerMount"`

	SizeLimited  bool  `yaml:"SizeLimited"`
	SizeLimitMiB int64 `yaml:"SizeLimitMiB"` // Size limit, if possible

	HostMounted bool   `yaml:"HostMounted"`
	HostMount   string `yaml:"HostMount"`

	Writable   bool `yaml:"Writable"`
	Executable bool `yaml:"Executable"`
}

// Keys is a convenience layer for SSH/GPG only (section 3): unlike a plain Volume it
// mounts the key read-only into the container home (.ssh for SSH, .gnupg for GPG).
type KeyType string

const (
	SSH KeyType = "SSH"
	GPG KeyType = "GPG"
)

type Key struct {
	Type KeyType `yaml:"Type"`
	Path string  `yaml:"Path"`
}

type AudioMeta struct {
	Pipewire   bool `yaml:"Pipewire"`
	LegacyALSA bool `yaml:"LegacyALSA"`
}

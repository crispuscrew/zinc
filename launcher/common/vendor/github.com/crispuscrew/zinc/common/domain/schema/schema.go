package schema

const SchemaVersion = 4

type Type string

const (
	ZincContainer      Type = "ZincContainer"
	ZincVirtualization Type = "ZincVirtualization"
)

// AppConfig is one app definition: ~/.config/zinc/apps/<name>.yaml
type AppConfig struct {
	SchemaVersion 		int  				`yaml:"SchemaVersion"`
	Type          		Type 				`yaml:"Type"`	
	AppNameID 			string		 		`yaml:"AppNameID"` 				// Also using as container/vm name
	Inherits 			string 				`yaml:"Inherits"`				// AppNameID of parent app

	LauncherMeta		LauncherMeta		`yaml:"LauncherMeta"`

	StartConditions		StartConditions		`yaml:"StartConditions"`
	StopConditions		StopConditions		`yaml:"StopConditions"`
	MinimizeFingerprint	bool				`yaml:"MinimizeFingerprint"`	// Reduce obvios fingerprint of container or VM on a best-efforts

	ResourcesMeta		ResourcesMeta		`yaml:"ResourcesMeta"`
	InternalUserMeta	InternalUserMeta 	`yaml:"InternalUserMeta"`
	ImageMeta			ImageMeta			`yaml:"ImageMeta"`
	DisplayMeta			DisplayMeta			`yaml:"DisplayMeta"`
	NetworkMeta			NetworkMeta			`yaml:"NetworkMeta"`
	NotificationMeta	NotificationMeta	`yaml:"NotificationMeta"`
	DBusMeta			DBusMeta			`yaml:"DBusMeta"`

	Configs				[]ConfigFile		`yaml:"Configs"`				// files the app ships with, from its own bundle
	Volumes				[]Volume			`yaml:"Volumes"`				// extra host bind mounts can also be added for one run via `zcr run -v` (not persisted here)
	Keys				[]Key				`yaml:"Keys"`
	HostTheme			bool				`yaml:"HostTheme"`
	AudioMeta			AudioMeta			`yaml:"AudioMeta"`

	CreatorFlags		[]string			`yaml:"CreatorFlags"`
	RunnerFlags			[]string			`yaml:"RunnerFlags"`
}

type LauncherMeta struct {
	Icon        	string `yaml:"Icon"`
	Description 	string `yaml:"Description"`
	Group       	string `yaml:"Group"` 		// optional category, presentation-only
}

type StartConditions struct {
	DependsOn 			[]string			`yaml:"DependsOn"`			// if there apps not running before, it launch it 

	Entrypoint			string				`yaml:"Entrypoint"`			// if empty use app default
	EntrypointEnv		map[string]string	`yaml:"EntrypointEnv"`

	Terminal			bool				`yaml:"Terminal"`			// if true, create terminal window for it
	Attached			bool				`yaml:"Attached"`			// createable attached terminals, every terminal hold container to live
	AttachedEntrypoint	string				`yaml:"AttachedEntrypoint"`	// if empty use Entrypoint
	AttachedEnv			map[string]string	`yaml:"AttachedEnv"`

	ReadOnlyRootfs		bool				`yaml:"ReadOnlyRootfs"`		// Prevent runtime changes to the app's base filesystem

// VM only
	LoaderBIOS			bool				`yaml:"LoaderBIOS"`			// Use BIOS instead UEFI
	SecureBoot			bool				`yaml:"SecureBoot"`
	TPM					bool				`yaml:"TPM"`
}

type StopConditions struct {
	KeepAlive	bool	`yaml:"KeepAlive"`		// Stays freeze/alive after entrypoint finish
	Background	bool	`yaml:"Background"`		// Stays alive after window close
	Autorestart	bool	`yaml:"Autorestart"`	// Autorestart if falls, not restart if manually closed
}

type ResourcesMeta struct {
	MaxCPUCores	float64	`yaml:"MaxCPUCores"`	// Containers may use fractions; VMs require whole cores
	MaxRamMiB	int64	`yaml:"MaxRamMiB"`		// maximum memory visible to the app
	PIDsLimit	int64	`yaml:"PIDsLimit"`		// For fork-bomb in containers prevented
}

type InternalUserMeta struct {
	UseNonRootUser	bool	`yaml:"UseNonRootUser"`		// If true using NonRootUser
	KeepUserID		bool	`yaml:"KeepUserID"`			// Keep the same id and etc as real host user
	NonRootUserName	string	`yaml:"NonRootUserName"`
}

type ImageMeta struct {
	Image 				string 		`yaml:"Image"`		// digest-pinned or base disk path
	Install 			[]string	`yaml:"Install"`	// RUN layers or runcmd
	SourceTag			string 		`yaml:"SourceTag"`	// tag the digest/immutable was resolved from. Empty for a hand-pinned

// VM only
	CloudInit			bool		`yaml:"CloudInit"`
	PublicSSHKeyPath	string		`yaml:"PublicSSHKeyPath"`
}

type DisplayMeta struct {
	DisableGpuAccess		bool	`yaml:"DisableGpuAccess"`		// GPU access / acceleration
	DisplayWidth			int		`yaml:"DisplayWidth"`
	DisplayHeight			int		`yaml:"DisplayHeight"`

	DisableSecurityContext	bool	`yaml:"DisableSecurityContext"` // security-context | passthrough
	RequireSecurityContext	bool	`yaml:"RequireSecurityContext"`

	Vulkan					bool	`yaml:"Vulkan"`					// Enable Vulkan GPU acceleration
}

type DBusMeta struct {
    Talk	[]string	`yaml:"Talk"`	// Bus names the app may call. A ".*" includes matching subnames.
    Own		[]string	`yaml:"Own"`	// Bus names the app may claim. A ".*" includes matching subnames.
}

type NetworkMeta struct {
    Interfaces			[]NetworkInterface	`yaml:"Interfaces"`
	RulesByPriority		[]NetworkRule		`yaml:"RulesByPriority"`
    DNS					DNSMeta				`yaml:"DNS"`
}

type NetworkInterface struct {
    // ID is a Zinc-side identifier, not the interface name visible inside the app.
    ID			string	`yaml:"ID"`			// zinc-side id, not interface name
    MacAddress	string	`yaml:"MacAddress"`	// generated if empty
}

type NetworkRule struct {
	From 			NetworkPeer			`yaml:"From"`
    To				NetworkPeer			`yaml:"To"`

    Domains			[]string			`yaml:"Domains"`
    Protocols		[]NetworkProtocol	`yaml:"Protocols"`

	AllowAllExcept	bool				`yaml:"AllowAllExcept"`
}

type NetworkPeer struct {
	Type		NetworkPeerType		`yaml:"Type"`
	AppNameID	string				`yaml:"AppNameID"`	// For App NetworkPeerType
	Interface	string				`yaml:"Interface"`	// host interface if host, interfaceID if Self and App NetworkPeerType
	Filter		NetworkPeerFilter	`yaml:"Filter"`
}

type NetworkPeerType string
const (
    NetworkPeerSelf		NetworkPeerType = "Self"
    NetworkPeerApp		NetworkPeerType = "App"
    NetworkPeerAnyApp	NetworkPeerType = "AnyApp"
    NetworkPeerHost		NetworkPeerType = "Host"
    NetworkPeerInternet	NetworkPeerType = "Internet"
    NetworkPeerAny		NetworkPeerType = "Any"
)

type NetworkPeerFilter struct {
    IPv4CIDR		[]string			`yaml:"IPv4CIDR"`
    IPv6CIDR		[]string			`yaml:"IPv6CIDR"`
	Ports			[]int				`yaml:"Ports"`
}

type NetworkProtocol string
const (
    NetworkTCP    NetworkProtocol = "TCP"
    NetworkUDP    NetworkProtocol = "UDP"
    NetworkICMP   NetworkProtocol = "ICMP"
    NetworkICMPv6 NetworkProtocol = "ICMPv6"
    NetworkSCTP   NetworkProtocol = "SCTP"
    NetworkGRE    NetworkProtocol = "GRE"
    NetworkESP    NetworkProtocol = "ESP"
    NetworkAH     NetworkProtocol = "AH"
)

type DNSMeta struct {
    ResolversByPriority	[]DNSResolver	`yaml:"ResolversByPriority"`
}

type DNSResolver struct {
    Protocol		DNSProtocol	`yaml:"Protocol"`
    Endpoint		string		`yaml:"Endpoint"`
    Path			string		`yaml:"Path"`
    BootstrapIPs	[]string	`yaml:"BootstrapIPs"`
}

type DNSProtocol string
const (
    DNSUDP		DNSProtocol = "UDP"
    DNSTCP		DNSProtocol = "TCP"
    DNSTLS		DNSProtocol = "TLS"
    DNSHTTPS	DNSProtocol = "HTTPS"
    DNSQUIC		DNSProtocol = "QUIC"
)

type NotificationMeta struct {
	Disabled			bool	`yaml:"Disabled"`
	Silenced			bool	`yaml:"Silenced"`			// All notification from app will be silenced

	UseCustomPrefix		bool	`yaml:"UseCustomPrefix"`
	CustomPrefix		string	`yaml:"CustomPrefix"`

	AllowedActions		bool	`yaml:"AllowedActions"`
	AllowedProlonged	bool	`yaml:"AllowedProlonged"`	// Allowed expire_timeout > cfg.notifications.Prolonged
	AllowedLinks		bool	`yaml:"AllowedLinks"`
}

type ConfigFile struct {
	BundlePath	string	`yaml:"BundlePath"`	// relative to $XDG_CONFIG_HOME/zinc/apps/<app>/configs.
	InnerMount	string	`yaml:"InnerMount"`
	Writable	bool	`yaml:"Writable"`
}

type Volume struct {
	InnerMount 		string	`yaml:"InnerMount"`

	SizeLimited		bool	`yaml:"SizeLimited"`
	SizeLimitMiB	int64	`yaml:"SizeLimitMiB"`
	HostMounted		bool	`yaml:"HostMounted"`
	HostMount		string	`yaml:"HostMount"`

	Writable		bool	`yaml:"Writable"`
	Executable		bool	`yaml:"Executable"`
}

type KeyType string
const (
	SSH	KeyType = "SSH"
	GPG	KeyType = "GPG"
)
type Key struct {	// Mounted read-only into the app user home (.ssh for SSH, .gnupg for GPG).
	Type	KeyType	`yaml:"Type"`
	Path	string 	`yaml:"Path"`
}

type AudioMeta struct {
	Playback	AudioDevice	`yaml:"Playback"`
	Microphone	AudioDevice	`yaml:"Microphone"`
	Monitor		AudioDevice	`yaml:"Monitor"`
}

type AudioDevice struct {
    PipeWireDefault	bool		`yaml:"PipeWireDefault"`
    PipeWireDevices	[]string	`yaml:"PipeWireDevices"`
    ALSADevices		[]string	`yaml:"ALSADevices"`
}

package schema

// SchemaVersion is the only app-config schema version this build understands.
const SchemaVersion = 2

type Type string
const (
	ZincContainer			Type
	//ZincVirtualization	Type
)

// AppConfig is one app definition: ~/.config/hyprzinc/apps/<name>.yaml 
type AppConfig struct {
	SchemaVersion 		int					`yaml:"SchemaVersion"`
	Type				Type				`yaml:"Type"`	// VM vs Container

	AppNameID			string				`yaml:"AppNameID"`		// Also using as container/vm name
	Icon				string				`yaml:"Icon"`
	Description			string				`yaml:"Description"`

	StartConditions 	StartConditions		`yaml:"StartConditions"`
	StopConditions		StopConditions		`yaml:"StopConditions"`

	ResourcesMeta		ResourcesMeta		`yaml:"ResourcesMeta"`
	InternalUserMeta	InternalUserMeta	`yaml:"InternalUserMeta"`
	ImageMeta			ImageMeta			`yaml:"ImageMeta"`
	DisplayMeta			DisplayMeta			`yaml:"DisplayMeta"`
	NetworkMeta			NetworkMeta			`yaml:"NetworkMeta"`
	NotificationMeta	NotificationMeta	`yaml:"NotificationMeta"`

	Volumes				[]Mount				`yaml:"Mounts"`
	Keys				[]Key				`yaml:"Keys"`
	HostTheme			bool				`yaml:"HostTheme"`
	AudioMeta			AudioMeta			`yaml:"AudioMeta"`
	Capabilities  		[]string			`yaml:"Capabilities"`	// if container --cap-add entries
}

type StartConditions struct {
	DependsOn				[]string	`yaml:"DependsOn"`	// apps, which must be running while/starting with it 

	Autorestart				bool		`yaml:"Autorestart"`	// Autorestart if falls, not restart if manually closed

	Entrypoint				string		`yaml:"Entrypoint"`	// if empty use app default
	Terminal				bool		`yaml:"Terminal"`	// if yes, create terminal window for it
	Multiterminal			bool		`yaml:"Multiterminal"`	// createable attached terminals, every terminal hold container to live
	MultiterminalEntrypoint	string		`yaml:"MultiterminalEntrypoint"`	// if empty use Entrypoint
}

type StopConditions struct {
	KeepAlive	bool	`yaml:"KeepAlive"`	// Stays container freeze/alive after entrypoint finish
	Background	bool	`yaml:"Background"`
}

type ResourcesMeta struct {

}

type InternalUserMeta struct {
	UseNonRootUser	bool	`yaml:"UseNonRootUser"`	// If true using NonRootUser
	KeepUserId		bool	`yaml:"KeepUserId"`	// Keep the same id and etc as real host user
	NonRootUserName	string	`yaml:"NonRootUserName"`
}

type ImageMeta struct {
	Image		string 		`yaml:"Image"`	// iso or container path
	Install		[]string	`yaml:"Install"`	// RUN and etc commands in build
}

type DisplayMeta struct {
	DisableSecurityContext	bool	`yaml:"DisableSecurityContext"`// security-context | passthrough
	DisableGpuAccess     	bool	`yaml:"DisableGpuAccess"`
}

type NetworkMeta struct {
	NetworkLists	[]NetworkList	`yaml:"NetworkLists"`
}

type NetworkList struct {
	Host			bool		`yaml:"Host"`	// it list for host or container?
	ContainerName	string		`yaml:"ContainerName"`	// if host == false, which container do we use::
	Interface		string		`yaml:"Interface"`
	BlockDNS		bool		`yaml:"BlockDNS`
	
	Blacklist		bool		`yaml:"Blacklist"`	// or whitelist

	IPv4CIDR		[]string	`yaml:"IPv4CIDR"`
	IPv6CIDR		[]string	`yaml:"IPv6CIDR"`
	Ports			[]int		`yaml:"Ports"`
}

type NotificationMeta struct {
	Disabled			bool 	`yaml:"Disabled"`
	Silented			bool	`yaml:"Silented"`	// All notification from app will be silented
	
	UseCustomPrefix		bool	`yaml:"UseCustomPrefix"`
	CustomPrefix		string	`yaml:"CustomPrefix"`

	AllowedActions		bool	`yaml:"AllowedActions"`
	AllowedProlonged	bool	`yaml:"AllowedProlonged"`	// Allowed expire_timeout > cfg.notifications.Prolonged
	AllowedLinks		bool	`yaml:"AllowedLinks"`
}

// Readable drop, because u cannot mount something u cannot read
type Volume struct {
	HostVolume		string	`yaml:"HostMount"`
	InnerMount	string	`yaml:"ContainerMount"`
	Writeable		bool	`yaml:"Writeable"`
	Executable		bool	`yaml:"Executable"`
}

// Keys is a convenience layer for SSH/GPG only (§3 [keys]): unlike [[mounts]] it
// also wires the agent socket and enforces 0600 inside the container.
type KeyType string
const (
	SSH	KeyType	= "SSH"
	GPG	KeyType	= "GPG"
)
type Key struct {
	Type	KeyType	`yaml:"Type"`
	Path	string	`yaml:"Path"`
}

type AudioMeta struct {
	Pipewire	bool	`yaml:"Pipewire"`
	LegacyALSA	bool	`yaml:"LegacyALSA"`
}

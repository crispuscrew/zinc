package options

// HostOptions carries the host-side values a launch needs - Wayland/runtime
// sockets, the theme bundle, the terminal emulator, the netfilter image, and the
// container-side home. They are passed explicitly so the argv-building adapters
// never read the environment themselves and stay pure/testable. Empty fields
// disable the corresponding wiring. The host adapter (adapters/host) resolves these
// from the environment; tests and dry-runs construct them directly.
type HostOptions struct {
	RuntimeDir     string   // host XDG_RUNTIME_DIR (wayland/pipewire sockets)
	WaylandDisplay string   // host WAYLAND_DISPLAY, e.g. "wayland-1"
	ThemeBundleDir string   // host path to the generated curated theme bundle (section 5.6)
	NetfilterImage string   // image carrying nft for the pasta lock-down step (section 5.3); empty → adapter default
	HomeDir        string   // container-side home for key mounts (.ssh/.gnupg); empty → /root
	Terminal       []string // terminal-emulator argv for terminal apps, e.g. ["foot"] or ["xterm","-e"] (section 11)
}

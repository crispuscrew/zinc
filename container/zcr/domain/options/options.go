package options

// HostOptions carries the host-side values a launch needs — Wayland/runtime
// sockets, the theme bundle, the terminal emulator, the netfilter image. They are
// passed explicitly so the argv-building adapters never read the environment
// themselves and stay pure/testable. Empty fields disable the corresponding wiring.
// The host adapter (core/adapters/host) resolves these from the environment; tests
// and dry-runs construct them directly. (Was runspec.Options before the hexagon.)
type HostOptions struct {
	RuntimeDir     string   // host XDG_RUNTIME_DIR (wayland/pipewire sockets)
	WaylandDisplay string   // host WAYLAND_DISPLAY, e.g. "wayland-1"
	ThemeBundleDir string   // host path to the generated curated theme bundle (§5.6)
	NetfilterImage string   // image carrying nft for the pasta lock-down step (§5.3); empty → adapter default
	Terminal       []string // terminal-emulator argv for app.terminal apps, e.g. ["foot"] or ["xterm","-e"] (§11)
}

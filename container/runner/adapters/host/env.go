// Package host is the environment adapter: it resolves the host-side launch options
// (Wayland/runtime sockets, theme bundle, terminal emulator, netfilter image) from
// environment variables into an options.HostOptions. It is the one place env → options
// lives, so every front-end wires the host identically and the argv-building adapters
// stay pure (docs/architecture.md section 9.1, section 13).
package host

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

// Options resolves the host launch options from the environment. NetfilterImage is
// left empty when unset, so the enforcer falls back to its built-in default.
func Options() options.HostOptions {
	return options.HostOptions{
		RuntimeDir:     os.Getenv("XDG_RUNTIME_DIR"),
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		ThemeBundleDir: os.Getenv("ZINC_THEME_BUNDLE"),
		HomeDir:        "/root",
		NetfilterImage: os.Getenv("ZINC_NETFILTER_IMAGE"),
		Terminal:       terminalArgv(),
		SessionBusPath: sessionBusPath(),
	}
}

// sessionBusPath resolves the host session bus socket for the D-Bus proxy (DBusMeta).
//
// Only the "unix:path=" form is understood, and anything else resolves to empty rather than
// being guessed at. DBUS_SESSION_BUS_ADDRESS is a comma-separated list of addresses in
// several transports (unix:, tcp:, autolaunch:, and unix:abstract= among them), and the proxy
// needs a filesystem socket it can bind-mount. An abstract socket has no path to mount, and a
// tcp bus is not something to hand a sandbox by inference. Empty makes the launch of an app
// that asked for a bus fail and say so, which is the fail-closed answer; the alternative -
// starting it with no bus - looks like the app is broken.
//
// The fallback is the standard rootless location, $XDG_RUNTIME_DIR/bus, which is where the
// per-user bus lives when the variable is unset (a login shell that never sourced the session
// environment, which is exactly the case for an app launched from a hotkey).
func sessionBusPath() string {
	for _, addr := range strings.Split(os.Getenv("DBUS_SESSION_BUS_ADDRESS"), ",") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(addr), "unix:path="); ok {
			return path
		}
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "bus")
	}
	return ""
}

// terminalArgv resolves the terminal emulator for terminal apps: $ZINC_TERMINAL, else
// $TERMINAL, split on spaces so both "foot" and "xterm -e" work. Empty when neither is
// set - launching a terminal app then fails with a clear message.
func terminalArgv() []string {
	spec := os.Getenv("ZINC_TERMINAL")
	if spec == "" {
		spec = os.Getenv("TERMINAL")
	}
	return strings.Fields(spec)
}

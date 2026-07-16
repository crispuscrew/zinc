// Package host is the environment adapter: it resolves the host-side launch options
// (Wayland/runtime sockets, theme bundle, terminal emulator, netfilter image) from
// environment variables into an options.HostOptions. It is the one place env → options
// lives, so every front-end wires the host identically and the argv-building adapters
// stay pure (docs/architecture.md section 9.1, section 13).
package host

import (
	"os"
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
	}
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

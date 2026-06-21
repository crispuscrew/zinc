// Package host is the environment adapter: it resolves the host-side launch
// options (Wayland/runtime sockets, theme bundle, terminal emulator, netfilter
// image) from environment variables into a domain.HostOptions. It is the one place
// env → options lives, so every front-end (hzc, hzl) wires the host identically and
// the argv-building adapters stay pure (docs/architecture.md §9.1, §13).
package host

import (
	"os"
	"strings"

	"github.com/crispuscrew/hyprzinc/core/domain"
)

// Options resolves the host launch options from the environment. NetfilterImage is
// left empty when unset, so the pasta enforcer falls back to its built-in default.
func Options() domain.HostOptions {
	return domain.HostOptions{
		RuntimeDir:     os.Getenv("XDG_RUNTIME_DIR"),
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		ThemeBundleDir: os.Getenv("HYPRZINC_THEME_BUNDLE"),
		HomeDir:        "/root",
		NetfilterImage: os.Getenv("HYPRZINC_NETFILTER_IMAGE"),
		Terminal:       terminalArgv(),
	}
}

// terminalArgv resolves the terminal emulator for app.terminal apps:
// $HYPRZINC_TERMINAL, else $TERMINAL, split on spaces so both "foot" and "xterm -e"
// work. Empty when neither is set — launching a terminal app then fails with a
// clear message.
func terminalArgv() []string {
	spec := os.Getenv("HYPRZINC_TERMINAL")
	if spec == "" {
		spec = os.Getenv("TERMINAL")
	}
	return strings.Fields(spec)
}

package waylandctx

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/paths"
)

func hostOpts() options.HostOptions {
	return options.HostOptions{RuntimeDir: "/run/user/1000", WaylandDisplay: "wayland-1"}
}

// The layout mirrors the D-Bus one (dbusproxy.HostSocketDir) a segment over, and the segment
// that varies is the RUNTIME name - so two instances of one app never share a socket. Sharing
// one would give them one identity and make instance_id a lie.
func TestSocketPathIsPerInstance(t *testing.T) {
	opt := hostOpts()
	work := SocketPath(opt.RuntimeDir, opt.WaylandDisplay, paths.Address{App: "browser", Instance: "work"})
	personal := SocketPath(opt.RuntimeDir, opt.WaylandDisplay, paths.Address{App: "browser", Instance: "personal"})
	bare := SocketPath(opt.RuntimeDir, opt.WaylandDisplay, paths.Address{App: "browser"})

	if want := "/run/user/1000/zinc/wayland/browser.work/wayland-1"; work != want {
		t.Fatalf("instance socket: got %q want %q", work, want)
	}
	if want := "/run/user/1000/zinc/wayland/browser/wayland-1"; bare != want {
		t.Fatalf("un-instanced socket: got %q want %q", bare, want)
	}
	if work == personal {
		t.Fatal("two instances of one app must not share a socket")
	}
}

// An absolute WAYLAND_DISPLAY is legal and a compositor started outside the session's runtime
// dir sets one, so the basename is what names the derived socket.
func TestSocketPathWithAbsoluteDisplay(t *testing.T) {
	got := SocketPath("/run/user/1000", "/tmp/zinc-test/wayland-9", paths.Address{App: "editor"})
	if want := "/run/user/1000/zinc/wayland/editor/wayland-9"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// No runtime dir and no display means there is nowhere to put a socket - the caller mounts
// the compositor's own and says nothing.
func TestSocketPathNeedsBothHostFacts(t *testing.T) {
	addr := paths.Address{App: "editor"}
	if got := SocketPath("", "wayland-1", addr); got != "" {
		t.Fatalf("no runtime dir should give no socket, got %q", got)
	}
	if got := SocketPath("/run/user/1000", "", addr); got != "" {
		t.Fatalf("no display should give no socket, got %q", got)
	}
}

func TestApplies(t *testing.T) {
	on := schema.AppConfig{}
	off := schema.AppConfig{DisplayMeta: schema.DisplayMeta{DisableSecurityContext: true}}
	for _, tc := range []struct {
		name string
		cfg  schema.AppConfig
		opt  options.HostOptions
		want bool
	}{
		{"default app on a wayland session", on, hostOpts(), true},
		{"app opted out", off, hostOpts(), false},
		{"headless host", on, options.HostOptions{RuntimeDir: "/run/user/1000"}, false},
		{"no runtime dir", on, options.HostOptions{WaylandDisplay: "wayland-1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Applies(tc.cfg, tc.opt); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// Establish returns an empty path, and no error, for an app that opted out - so the argv
// builder mounts the compositor's own socket and labels the container honestly.
func TestEstablishSkipsWhenNotApplicable(t *testing.T) {
	cfg := schema.AppConfig{DisplayMeta: schema.DisplayMeta{DisableSecurityContext: true}}
	socket, err := Broker{}.Establish(paths.Address{App: "editor"}, cfg, hostOpts())
	if err != nil || socket != "" {
		t.Fatalf("got (%q, %v), want an empty path and no error", socket, err)
	}
}

// The holder reports on a pipe rather than stderr, so the one line it writes is a contract
// between two copies of the same binary. An unreadable line must be an error: reading it as
// "no context" would launch an app unprotected and say nothing.
func TestParseStatus(t *testing.T) {
	for _, tc := range []struct {
		name      string
		line      string
		socket    string
		supported bool
		wantErr   bool
	}{
		{name: "ready", line: "ok /run/user/1000/zinc/wayland/browser/wayland-1\n",
			socket: "/run/user/1000/zinc/wayland/browser/wayland-1", supported: true},
		{name: "compositor has no such protocol", line: "unsupported the compositor does not implement it\n"},
		{name: "holder failed", line: "error listen on /run/user/1000/...: permission denied\n", wantErr: true},
		{name: "success with no path", line: "ok\n", wantErr: true},
		{name: "garbage", line: "Segmentation fault\n", wantErr: true},
		{name: "empty", line: "\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			socket, supported, err := parseStatus(tc.line)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error: got %v, wantErr %v", err, tc.wantErr)
			}
			if socket != tc.socket || supported != tc.supported {
				t.Fatalf("got (%q, %v) want (%q, %v)", socket, supported, tc.socket, tc.supported)
			}
		})
	}
}

// The failure reason has to survive the pipe, or a broken holder is diagnosed as "something
// went wrong" and the launch error names nothing.
func TestParseStatusCarriesTheReason(t *testing.T) {
	_, _, err := parseStatus("error listen on /run/user/1000/zinc/wayland/x/wayland-1: address already in use\n")
	if err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("the holder's reason was lost: %v", err)
	}
}

func TestCompositorSocket(t *testing.T) {
	got, err := compositorSocket(hostOpts())
	if err != nil || got != "/run/user/1000/wayland-1" {
		t.Fatalf("got (%q, %v)", got, err)
	}
	got, err = compositorSocket(options.HostOptions{RuntimeDir: "/run/user/1000", WaylandDisplay: "/tmp/wl/wayland-3"})
	if err != nil || got != "/tmp/wl/wayland-3" {
		t.Fatalf("an absolute WAYLAND_DISPLAY must be honoured as-is, got (%q, %v)", got, err)
	}
	if _, err := compositorSocket(options.HostOptions{RuntimeDir: "/run/user/1000"}); err == nil {
		t.Fatal("no display means no compositor to ask")
	}
	if _, err := compositorSocket(options.HostOptions{WaylandDisplay: "wayland-1"}); err == nil {
		t.Fatal("a relative display with no runtime dir cannot be located")
	}
}

// Package dbusproxy is the D-Bus adapter: it implements ports.DBusBroker by running
// xdg-dbus-proxy in a container Zinc owns, so an app with DBusMeta gets a session bus
// carrying only the names its config named (docs/architecture.md section 5.7).
//
// The shape is the same one the egress lock-down uses: the dangerous thing is established
// outside the app and the app is handed only the filtered result. Two properties are load
// bearing, and both are about what the app can reach rather than what it is allowed to call.
//
// The proxy is NOT in the app's pod. A pod shares the PID namespace, so a proxy inside it
// would be a process the app could signal or ptrace - the filter and the thing being
// filtered, in one blast radius. It is a standalone container instead, and shares with the
// app exactly one thing: the socket, through a bind mount.
//
// The app never receives the real bus socket. Only the proxy mounts it, read-write because a
// bus client must write to connect, and the app's mount is the proxy's own socket.
//
// Everything here is argv-building and therefore pure and testable; the Runtime executes
// what this returns.
package dbusproxy

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

// DefaultImage carries xdg-dbus-proxy. It is the same helper image the netfilter steps use,
// referenced by local tag and run with --pull never (section 5.5): a locally built, vetted
// image, never something fetched at launch.
const DefaultImage = "zinc/netfilter:local"

// ctrPaths inside the proxy container. The real bus and the served socket are kept in
// separate directories so the mount that carries the host bus can never be the directory the
// app also mounts.
const (
	ctrHostBus  = "/run/zinc-host-bus" // the real session bus, proxy-only
	ctrProxyDir = "/run/zinc-proxy"    // where the proxy writes the filtered socket
	proxySocket = "bus"                // socket filename, in both the proxy and the app
)

// ctrAppSocket is where the filtered socket lands in the APP container. It sits outside
// /run/zinc (the XDG runtime dir the Wayland and Pipewire sockets share) because
// DBUS_SESSION_BUS_ADDRESS names this path explicitly and nothing benefits from it being
// adjacent to sockets the app reaches by a different convention.
const ctrAppSocket = "/run/zinc-bus/bus"

// Broker implements ports.DBusBroker. The host facts are held rather than passed per call,
// so Teardown is reachable from Stop, which knows an app config and nothing about the host.
//
//   - Image: the helper carrying xdg-dbus-proxy; empty means DefaultImage.
//   - RuntimeDir: host XDG_RUNTIME_DIR, the parent of every app's socket directory.
//   - SessionBusPath: the real session bus socket. Only the proxy ever sees it.
type Broker struct {
	Image          string
	RuntimeDir     string
	SessionBusPath string
}

// New builds a Broker from the resolved host options. image may be empty (DefaultImage).
func New(image string, opt options.HostOptions) Broker {
	return Broker{Image: image, RuntimeDir: opt.RuntimeDir, SessionBusPath: opt.SessionBusPath}
}

func (brk Broker) image() string {
	if strings.TrimSpace(brk.Image) == "" {
		return DefaultImage
	}
	return brk.Image
}

// ContainerName is the proxy container's name, derived from the app's. It is a separate
// object in the runtime's namespace from the app itself, so it needs a name that cannot
// collide with an app's (an app name is [a-z0-9][a-z0-9._-]*, so a "zinc-dbus-" prefix is
// unambiguous) and is recoverable from the app name alone at teardown.
func ContainerName(app string) string { return "zinc-dbus-" + app }

// HostSocketDir is the per-app directory on the host holding that app's filtered socket. Per
// app, not shared: two apps with different grants must not be able to reach each other's
// socket, and the whole point is that this one carries only what this app was given.
func HostSocketDir(runtimeDir, app string) string {
	if runtimeDir == "" {
		return ""
	}
	return filepath.Join(runtimeDir, "zinc", "dbus", app)
}

// RunFlags attaches the filtered socket to the app: the bind mount and the address pointing
// at it. Read-write, because connecting to a unix socket is a write operation - a read-only
// mount would present the app with a socket it cannot use.
func (brk Broker) RunFlags(cfg schema.AppConfig) []string {
	dir := HostSocketDir(brk.RuntimeDir, cfg.AppNameID)
	if cfg.DBusMeta.IsZero() || dir == "" {
		return nil
	}
	return []string{
		"-v", filepath.Join(dir, proxySocket) + ":" + ctrAppSocket + ":rw",
		"-e", "DBUS_SESSION_BUS_ADDRESS=unix:path=" + ctrAppSocket,
	}
}

// Prepare creates the app's socket directory and starts the proxy. It runs before the app, so
// the socket exists by the time the app looks for it.
//
// A missing host bus is an error rather than a silent skip: the app asked for bus access, and
// starting it with no bus at all would surface as the app being broken in a way that points
// nowhere near the config.
func (brk Broker) Prepare(cfg schema.AppConfig) ([]ports.Command, error) {
	if cfg.DBusMeta.IsZero() {
		return nil, nil
	}
	dir := HostSocketDir(brk.RuntimeDir, cfg.AppNameID)
	if dir == "" {
		return nil, fmt.Errorf("%s: DBusMeta needs XDG_RUNTIME_DIR set, to place the app's bus socket", cfg.AppNameID)
	}
	if strings.TrimSpace(brk.SessionBusPath) == "" {
		return nil, fmt.Errorf("%s: DBusMeta asked for a filtered session bus, but no host session bus could be resolved - set DBUS_SESSION_BUS_ADDRESS to a unix:path= address", cfg.AppNameID)
	}

	// The socket directory is created by a helper-image container rather than by this process
	// so that every step of a launch is one Command the Runtime executes and `zcr run` can
	// print. A launch that did some of its work as direct syscalls would not be fully visible
	// in a dry run, which is the property the plan output exists for.
	steps := []ports.Command{{
		Args: []string{
			"run", "--rm", "--pull", "never",
			"--userns=keep-id",
			"--security-opt", "no-new-privileges", "--cap-drop", "all",
			"-v", filepath.Dir(dir) + ":/run/zinc-dbus-root:rw",
			brk.image(),
			"mkdir", "-p", filepath.Join("/run/zinc-dbus-root", filepath.Base(dir)),
		},
		Desc: "create bus socket dir for " + cfg.AppNameID,
	}}

	// The proxy itself: detached, holding the real bus, serving the filtered one. --cap-drop
	// all and no-new-privileges apply to the proxy as much as to the app - it is a helper Zinc
	// runs, not a trusted component, and it needs no capability to relay a socket.
	//
	// keep-id is why validation requires it on the app too: the socket is created by this
	// container as the host uid, and an app in a different user namespace could not connect.
	proxyArgs := []string{
		"run", "-d", "--rm", "--pull", "never",
		"--name", ContainerName(cfg.AppNameID),
		"--userns=keep-id",
		"--security-opt", "no-new-privileges", "--cap-drop", "all",
		"--network", "none", // a bus relay needs no network, and this is the app's most privileged neighbour
		"-v", brk.SessionBusPath + ":" + ctrHostBus + ":rw",
		"-v", dir + ":" + ctrProxyDir + ":rw",
		brk.image(),
		"xdg-dbus-proxy",
		"unix:path=" + ctrHostBus,
		filepath.Join(ctrProxyDir, proxySocket),
		"--filter", // without this the proxy forwards everything and the grants below are decoration
	}
	proxyArgs = append(proxyArgs, FilterArgs(cfg.DBusMeta)...)
	steps = append(steps, ports.Command{Args: proxyArgs, Desc: "start filtered dbus proxy for " + cfg.AppNameID})
	return steps, nil
}

// FilterArgs renders DBusMeta as xdg-dbus-proxy filter options, Talk before Own. Exported so
// a test can assert the exact grants a config produces - the thing that decides what the app
// can reach - without reconstructing the whole launch.
//
// --filter is supplied by the caller, not here: it is the switch that makes these grants an
// allowlist rather than annotations on a fully open bus, so it belongs with the invocation
// that must not omit it.
func FilterArgs(bus schema.DBusMeta) []string {
	args := make([]string, 0, len(bus.Talk)+len(bus.Own))
	for _, name := range bus.Talk {
		args = append(args, "--talk="+name)
	}
	for _, name := range bus.Own {
		args = append(args, "--own="+name)
	}
	return args
}

// Teardown removes the proxy and the socket directory. Both, and in this order: the proxy is
// --rm so it usually removes itself, but "usually" leaves an app that cannot be relaunched
// because its proxy name is taken, and the directory outlives the proxy either way.
//
// `rm -f` rather than `stop`, because this must also clean up after a proxy that already
// exited, where stop would fail and stop the teardown before the directory was removed.
func (brk Broker) Teardown(cfg schema.AppConfig) []ports.Command {
	if cfg.DBusMeta.IsZero() {
		return nil
	}
	steps := []ports.Command{{
		Args: []string{"rm", "-f", "--ignore", ContainerName(cfg.AppNameID)},
		Desc: "remove dbus proxy for " + cfg.AppNameID,
	}}
	if dir := HostSocketDir(brk.RuntimeDir, cfg.AppNameID); dir != "" {
		steps = append(steps, ports.Command{
			Args: []string{
				"run", "--rm", "--pull", "never",
				"--userns=keep-id",
				"--security-opt", "no-new-privileges", "--cap-drop", "all",
				"-v", filepath.Dir(dir) + ":/run/zinc-dbus-root:rw",
				brk.image(),
				"rm", "-rf", filepath.Join("/run/zinc-dbus-root", filepath.Base(dir)),
			},
			Desc: "remove bus socket dir for " + cfg.AppNameID,
		})
	}
	return steps
}

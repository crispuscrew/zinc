package dbusproxy

import (
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

const hostBus = "/run/user/1000/bus"

func testOpt() options.HostOptions {
	return options.HostOptions{RuntimeDir: "/run/user/1000", SessionBusPath: hostBus}
}

func busApp() schema.AppConfig {
	return schema.AppConfig{
		AppNameID: "notes",
		DBusMeta: schema.DBusMeta{
			Talk: []string{"org.freedesktop.portal.Desktop"},
			Own:  []string{"org.mpris.MediaPlayer2.notes"},
		},
	}
}

// flat joins a command's args so a test can assert on substrings without caring where the
// argument boundaries fall.
func flat(cmd ports.Command) string { return strings.Join(cmd.Args, " ") }

// An app with no DBusMeta must get nothing at all: no socket, no bus address, no proxy. This
// is the fail-closed default, and the property most likely to rot, since every other test
// here sets DBusMeta.
func TestNoBusRequestedProducesNothing(t *testing.T) {
	brk := New("", testOpt())
	cfg := schema.AppConfig{AppNameID: "plain"}

	if flags := brk.RunFlags(cfg); flags != nil {
		t.Errorf("RunFlags on an app without DBusMeta = %v, want nil", flags)
	}
	steps, err := brk.Prepare(cfg)
	if err != nil || steps != nil {
		t.Errorf("Prepare on an app without DBusMeta = %v, %v; want nil, nil", steps, err)
	}
	if steps := brk.Teardown(cfg); steps != nil {
		t.Errorf("Teardown on an app without DBusMeta = %v, want nil", steps)
	}
}

// THE containment property: the real session bus is mounted into the proxy and must never
// appear anywhere in the app's own flags. If this fails, the app is talking to the unfiltered
// bus and every grant in DBusMeta is decoration.
func TestAppNeverReceivesTheRealBusSocket(t *testing.T) {
	brk := New("", testOpt())
	flags := strings.Join(brk.RunFlags(busApp()), " ")

	if strings.Contains(flags, hostBus) {
		t.Fatalf("the app's flags carry the real bus socket %q: %s", hostBus, flags)
	}
	if !strings.Contains(flags, "DBUS_SESSION_BUS_ADDRESS=unix:path="+ctrAppSocket) {
		t.Errorf("app is not pointed at the filtered socket: %s", flags)
	}
	// A bus client writes to connect, so a read-only mount would hand the app a socket it
	// cannot use - a failure that looks like a broken app rather than a wrong mount.
	if !strings.Contains(flags, ctrAppSocket+":rw") {
		t.Errorf("filtered socket is not mounted rw: %s", flags)
	}
}

// The proxy must not join the app's pod: a pod shares the PID namespace, which would let the
// app signal or ptrace the process filtering it.
func TestProxyIsNotInTheAppsPod(t *testing.T) {
	steps, err := New("", testOpt()).Prepare(busApp())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	for _, step := range steps {
		if slices.Contains(step.Args, "--pod") {
			t.Errorf("proxy step joins the app's pod: %s", flat(step))
		}
	}
}

// --filter is the switch that turns the grants into an allowlist. Without it xdg-dbus-proxy
// forwards everything, so its absence would silently open the whole bus.
func TestProxyRunsFilteredWithExactlyTheConfiguredGrants(t *testing.T) {
	steps, err := New("", testOpt()).Prepare(busApp())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	proxy := steps[len(steps)-2] // last step is the readiness wait
	args := flat(proxy)

	for _, want := range []string{
		"--filter",
		"--talk=org.freedesktop.portal.Desktop",
		"--own=org.mpris.MediaPlayer2.notes",
		"unix:path=" + ctrHostBus,
		hostBus + ":" + ctrHostBus,
		"--cap-drop all",
		"no-new-privileges",
		"--userns=keep-id",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("proxy args missing %q: %s", want, args)
		}
	}
	if !strings.Contains(args, "--name "+ContainerName("notes")) {
		t.Errorf("proxy is not named for its app: %s", args)
	}
}

// A grant the config did not make must not appear. Guards against a future edit that adds a
// convenience default (a portal, a notifier) that no config asked for.
func TestNoGrantsBeyondTheConfig(t *testing.T) {
	cfg := busApp()
	cfg.DBusMeta = schema.DBusMeta{Talk: []string{"org.example.One"}}

	got := FilterArgs(cfg.DBusMeta)
	want := []string{"--talk=org.example.One"}
	if !slices.Equal(got, want) {
		t.Errorf("FilterArgs = %v, want %v", got, want)
	}
}

// An app that asked for a bus when none can be resolved must fail the launch, not start
// without one - a silent skip surfaces inside the app as an unexplained connection error.
func TestMissingHostBusFailsClosed(t *testing.T) {
	brk := New("", options.HostOptions{RuntimeDir: "/run/user/1000"}) // no SessionBusPath
	if _, err := brk.Prepare(busApp()); err == nil {
		t.Fatal("Prepare with no resolvable host bus: want an error, got nil")
	}
}

// Same for a missing runtime dir: there is nowhere to put the socket, so the launch must say
// so rather than proceed.
func TestMissingRuntimeDirFailsClosed(t *testing.T) {
	brk := New("", options.HostOptions{SessionBusPath: hostBus}) // no RuntimeDir
	if _, err := brk.Prepare(busApp()); err == nil {
		t.Fatal("Prepare with no runtime dir: want an error, got nil")
	}
}

// Teardown must remove the proxy AND the socket directory. The proxy is --rm so it usually
// removes itself, but "usually" leaves an app that cannot relaunch because its proxy name is
// taken, and the directory outlives the proxy either way.
func TestTeardownRemovesProxyAndSocketDir(t *testing.T) {
	steps := New("", testOpt()).Teardown(busApp())
	if len(steps) != 2 {
		t.Fatalf("Teardown produced %d steps, want 2 (proxy, socket dir): %v", len(steps), steps)
	}
	if first := flat(steps[0]); !strings.Contains(first, "rm -f") || !strings.Contains(first, ContainerName("notes")) {
		t.Errorf("first teardown step does not force-remove the proxy: %s", first)
	}
	if second := flat(steps[1]); !strings.Contains(second, "notes") {
		t.Errorf("second teardown step does not target the app's socket dir: %s", second)
	}
}

// The app must not be allowed to start before the proxy answers. `podman run -d` returns when
// the container starts, not when xdg-dbus-proxy has bound and begun serving, so Prepare has to
// close that window itself - and it has to be the LAST step, after which the app runs.
func TestPrepareWaitsForTheProxyToServe(t *testing.T) {
	steps, err := New("", testOpt()).Prepare(busApp())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	last := flat(steps[len(steps)-1])
	if !strings.Contains(last, "dbus-send") {
		t.Errorf("the last pre-app step does not probe the bus: %s", last)
	}
	if !strings.Contains(last, "exit 1") {
		t.Errorf("the readiness probe does not fail closed: %s", last)
	}
}

// Two apps must not share a socket directory, or an app would reach a bus filtered for
// someone else's grants.
func TestSocketDirIsPerApp(t *testing.T) {
	one := HostSocketDir("/run/user/1000", "one")
	two := HostSocketDir("/run/user/1000", "two")
	if one == two || one == "" {
		t.Errorf("socket dirs are not per app: %q vs %q", one, two)
	}
}

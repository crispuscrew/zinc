//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (env environment) bus(t *testing.T) {
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		t.Skip("no XDG_RUNTIME_DIR for session-bus test")
	}
	busPath := filepath.Join(runtime, "bus")
	if _, err := os.Stat(busPath); err != nil {
		t.Skipf("no session bus: %v", err)
	}
	must(t, env.creator, "new", "busapp", "--image", appImage, "--entrypoint", "/sleeper.sh",
		"--dbus-talk", "org.freedesktop.portal.Desktop", "--dbus-own", "org.mpris.MediaPlayer2.busapp")
	authored, err := os.ReadFile(filepath.Join(env.apps, "busapp.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(authored), "KeepUserID: true") {
		t.Fatal("DBus authoring omitted KeepUserID")
	}
	defer tool(env.creator, "stop", "busapp")
	env.start(t, "busapp")
	proxy := "zinc-dbus-busapp"
	if !waitFor(func() bool { return env.running(proxy) }) {
		t.Fatal("DBus proxy did not start")
	}
	pod := must(t, "podman", "inspect", "--format", "{{.Pod}}", proxy)
	if strings.TrimSpace(pod) != "" {
		t.Fatalf("proxy shares app pod: %s", pod)
	}
	values := must(t, "podman", "inspect", "--format", "{{.Config.Env}}", "busapp")
	if !strings.Contains(values, "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/zinc-bus/bus") {
		t.Fatal(values)
	}
	mounts := must(t, "podman", "inspect", "--format", "{{range .Mounts}}{{.Source}} {{end}}", "busapp")
	if strings.Contains(mounts, busPath) {
		t.Fatalf("raw session bus mounted: %s", mounts)
	}
	env.busAttribution(t, proxy)
	must(t, env.creator, "stop", "busapp")
	if !waitFor(func() bool { return !env.running(proxy) }) {
		t.Fatal("DBus proxy survived stop")
	}
}

package podman

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

func TestExecEnvironmentAndTerminalWrapping(t *testing.T) {
	args := ExecArgs("demo", []string{"sh", "-c", "printf '%s' \"$VALUE\""}, map[string]string{"ZED": "", "VALUE": "a b;'$()"})
	want := []string{"exec", "-it", "-e", "VALUE=a b;'$()", "-e", "ZED=", "demo", "sh", "-c", "printf '%s' \"$VALUE\""}
	if !slices.Equal(args, want) {
		t.Fatal(args)
	}
	wrapped := TerminalLaunch([]string{"xterm", "-e"}, args, false)
	if !slices.Equal(wrapped, append([]string{"xterm", "-e", "podman"}, want...)) {
		t.Fatal(wrapped)
	}
	held := TerminalLaunch([]string{"foot"}, args, true)
	if len(held) != 4 || held[1] != "sh" || !strings.Contains(held[3], "read _") {
		t.Fatal(held)
	}
	// A shell round-trip proves hostile/empty argv remain data, independently of the renderer.
	values := []string{"", "a'b", "$(false)", "line\nbreak", "a b;exit 99"}
	output, err := exec.Command("sh", "-c", "printf '%s\\000' "+shellJoin(values)).Output()
	if err != nil || string(output) != strings.Join(values, "\x00")+"\x00" {
		t.Fatalf("%q: %v", output, err)
	}
}

func TestDetachedAppCommand(t *testing.T) {
	cfg := validCfg()
	for _, terminal := range []bool{false, true} {
		cfg.StartConditions.Terminal = terminal
		process, err := appCmd(cfg, options.HostOptions{Terminal: []string{"foot"}}, []string{"run", "image"})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"podman", "run", "image"}
		if terminal {
			want = append([]string{"foot"}, want...)
		}
		if !slices.Equal(process.Args, want) || process.SysProcAttr == nil || !process.SysProcAttr.Setsid {
			t.Fatal(process)
		}
	}
	if _, err := appCmd(cfg, options.HostOptions{}, nil); err == nil {
		t.Fatal("missing terminal accepted")
	}
}

func TestLifecycleCommandsAndDiagnostics(t *testing.T) {
	for _, pair := range []struct{ got, want []string }{
		{StopArgs("demo"), []string{"stop", "demo"}}, {RestartArgs("demo"), []string{"restart", "demo"}},
		{InspectArgs("demo"), []string{"inspect", "demo"}}, {LogsArgs("demo", true), []string{"logs", "-f", "demo"}},
		{LogsArgs("demo", false), []string{"logs", "demo"}},
	} {
		if !slices.Equal(pair.got, pair.want) {
			t.Fatal(pair)
		}
	}
	command := ports.Command{Args: []string{"run", "localhost/zinc/netfilter:local"}}
	if !strings.Contains(helperImageHint(command, []byte("image not known")), "make -C container/runner netfilter-image") {
		t.Fatal("missing helper hint")
	}
	if helperImageHint(command, []byte("syntax error")) != "" {
		t.Fatal("unrelated hint")
	}
	command.Args = []string{"run", "docker.io/library/alpine"}
	if helperImageHint(command, []byte("image not known")) != "" {
		t.Fatal("app got helper hint")
	}
	pids := parsePIDs("notes 4001\nbroken\nempty \nzero 0\nproxy 4002\n")
	if len(pids) != 2 || pids["notes"] != 4001 || pids["proxy"] != 4002 {
		t.Fatal(pids)
	}
	if len(parsePIDs("\n")) != 0 {
		t.Fatal("empty pid table")
	}
}

package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/container/runner/ports"
)

func TestPlanShellRoundTrip(t *testing.T) {
	args := []string{"run", "--env", "VALUE=a'b;$(exit 99)", "", "line\nbreak", "image"}
	plan := captureStdout(t, func() {
		printPlan([]ports.Command{{Args: args, Stdin: "literal $HOME `false`\nZINC_STDIN\n", Desc: "safe\ncomment"}})
	})
	script := "podman() { printf '%s\\000' \"$@\"; cat; };\n" + plan
	output, err := exec.Command("sh", "-c", script).Output()
	want := strings.Join(args, "\x00") + "\x00literal $HOME `false`\nZINC_STDIN\n"
	if err != nil || string(output) != want {
		t.Fatalf("rendered plan changed argv/stdin: %q %v\n%s", output, err, plan)
	}
}

func TestLegacyVMUnrepresentableSettingsFailMigration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := writeApp(t, "SchemaVersion: 3\nType: ZincVirtualization\nAppNameID: guest\nVirtualizationMeta: {BaseDigest: sha256:abc, Display: None}\n")
	for _, command := range []string{"run", "validate", "stop", "inspect"} {
		err := run([]string{command, path})
		if err == nil || !strings.Contains(err.Error(), "cannot be represented") {
			t.Errorf("%s: wanted migration refusal, got %v", command, err)
		}
	}
}

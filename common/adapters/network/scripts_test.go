package network

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func scriptTools(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	log := filepath.Join(root, "commands")
	tools := map[string]string{
		"readlink": "case \"$1\" in */net) echo 'net:[123]';; */user) echo 'user:[456]';; esac",
		"ip":       "exit 0",
		"nft":      "body=$(cat); case \"$body\" in *FAIL*) echo failed >> \"$RECORD\"; exit 9;; *'rule[0]'*) echo policy >> \"$RECORD\";; *) echo closed >> \"$RECORD\";; esac",
		"guest":    "echo guest >> \"$RECORD\"; exit \"${GUEST_STATUS:-0}\"",
		"setpriv":  "while [ \"$1\" != -- ]; do shift; done; shift; exec \"$@\"",
	}
	for name, body := range tools {
		if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root, log
}

func executeScript(t *testing.T, script, input, status string) (string, error) {
	t.Helper()
	root, log := scriptTools(t)
	command := exec.Command("sh", "-c", script)
	command.Env = append(os.Environ(), "PATH="+root+":"+os.Getenv("PATH"), "RECORD="+log, "GUEST_STATUS="+status)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if len(output) > 0 {
		t.Log(string(output))
	}
	recorded, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(recorded), err
}

func TestRecordedStartupAndRollback(t *testing.T) {
	_, manifest := manifestFixture()
	script := RunScript(manifest, []string{"guest"})
	for _, flag := range []string{"setpriv --no-new-privs", "--bounding-set=-all", "--inh-caps=-all", "--ambient-caps=-all"} {
		if !strings.Contains(script, flag) {
			t.Fatalf("guest privilege drop missing %s", flag)
		}
	}
	for _, testCase := range []struct {
		input, status, want string
		failed              bool
	}{
		{"rule[0]", "0", "closed\npolicy\nguest\nclosed\n", false},
		{"rule[0]", "7", "closed\npolicy\nguest\nclosed\n", true},
		{"FAIL", "0", "closed\nfailed\nclosed\n", true},
	} {
		got, err := executeScript(t, RunScript(manifest, []string{"guest"}), testCase.input, testCase.status)
		if got != testCase.want || (err != nil) != testCase.failed {
			t.Fatalf("recorded %q, error %v; want %q failure=%v", got, err, testCase.want, testCase.failed)
		}
	}
}

func TestRecordedInitializerLeavesPolicyOnlyOnSuccess(t *testing.T) {
	_, manifest := manifestFixture()
	got, err := executeScript(t, ApplyScript(manifest), "rule[0]", "0")
	if err != nil || got != "closed\npolicy\n" {
		t.Fatalf("successful initializer: %q %v", got, err)
	}
	got, err = executeScript(t, ApplyScript(manifest), "FAIL", "0")
	if err == nil || got != "closed\nfailed\nclosed\n" {
		t.Fatalf("rollback: %q %v", got, err)
	}
}

func TestPreflightPinsIdentitiesAndDeterministicDeviceOrder(t *testing.T) {
	_, manifest := manifestFixture()
	manifest.Topology.ExternalInterfaces = []string{"zext0", "aext0"}
	script := Preflight(manifest)
	if !strings.Contains(script, "user:[456]") || !strings.Contains(script, "net:[123]") || strings.Index(script, "aext0") > strings.Index(script, "zext0") {
		t.Fatal("unsafe preflight")
	}
	for repeat := 0; repeat < 10; repeat++ {
		if script != Preflight(manifest) {
			t.Fatal("nondeterministic preflight")
		}
	}
}

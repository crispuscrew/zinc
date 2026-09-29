package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/store"
)

func TestNewAuthorsCanonicalSharedFieldsAndWarns(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	output, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = output
	t.Cleanup(func() { os.Stdout = previous; output.Close() })
	err = run([]string{"new", "app", "--image", "localhost/app:local", "--desc", "description", "--icon", "icon", "--group", "Work",
		"--entrypoint", "/bin/sh", "--terminal", "--attached", "--attached-entrypoint", "/bin/sh", "--autorestart", "--read-only-rootfs",
		"--env", "TOKEN=a=b c", "--attached-env", "SESSION=private", "--creator-flag=--label", "--creator-flag=two words",
		"--runner-flags", "['--label', '$(literal)']", "--playback-default", "--microphone-pipewire", "alsa_input.exact-name"})
	if err != nil {
		t.Fatal(err)
	}
	sto, _ := store.Default()
	cfg, err := sto.Load("app")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LauncherMeta.Group != "Work" || !cfg.StartConditions.Attached || !cfg.StopConditions.Autorestart || !cfg.StartConditions.ReadOnlyRootfs {
		t.Fatal(cfg)
	}
	if cfg.StartConditions.EntrypointEnv["TOKEN"] != "a=b c" || cfg.StartConditions.AttachedEnv["SESSION"] != "private" {
		t.Fatal(cfg.StartConditions)
	}
	if !reflect.DeepEqual(cfg.CreatorFlags, []string{"--label", "two words"}) || !reflect.DeepEqual(cfg.RunnerFlags, []string{"--label", "$(literal)"}) {
		t.Fatal("argv boundaries changed")
	}
	if !cfg.AudioMeta.Playback.PipeWireDefault || cfg.AudioMeta.Microphone.PipeWireDevices[0] != "alsa_input.exact-name" {
		t.Fatal(cfg.AudioMeta)
	}
	data, err := os.ReadFile(output.Name())
	if err != nil || !strings.Contains(string(data), "Raw backend flags") {
		t.Fatalf("missing warning: %s %v", data, err)
	}
	yaml, err := os.ReadFile(sto.Path("app"))
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"\nDescription:", "Multiterminal", "\nEnv:", "VirtualizationMeta:", "Capabilities:"} {
		if strings.Contains(string(yaml), legacy) {
			t.Errorf("wrote legacy key %s", legacy)
		}
	}
}

func TestNewVMForwardAuthorsOnlyExplicitHostIngress(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)
	if err := run([]string{"new", "guest", "--vm", "--image", "/images/base.qcow2", "--base-digest", testDigest, "--forward", "2222:22"}); err != nil {
		t.Fatal(err)
	}
	sto, _ := store.Default()
	cfg, err := sto.Load("guest")
	if err != nil {
		t.Fatal(err)
	}
	options, err := sto.LoadVM(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.NetworkMeta.Interfaces) != 1 || len(cfg.NetworkMeta.RulesByPriority) != 1 {
		t.Fatal(cfg.NetworkMeta)
	}
	rule := cfg.NetworkMeta.RulesByPriority[0]
	if rule.From.Type != schema.NetworkPeerHost || rule.To.Type != schema.NetworkPeerSelf || rule.To.Filter.Ports[0] != 22 || rule.Protocols[0] != schema.NetworkTCP {
		t.Fatal(rule)
	}
	if options.ForwardPorts[0].BindAddress != "127.0.0.1" || options.ForwardPorts[0].HostPort != 2222 {
		t.Fatal(options)
	}
}

package qemu

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
)

func fixture() (schema.AppConfig, Layout) {
	cfg := schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincVirtualization, AppNameID: "guest",
		ImageMeta: schema.ImageMeta{Image: "/images/base.qcow2"}, ResourcesMeta: schema.ResourcesMeta{MaxRamMiB: 2048, MaxCPUCores: 2}}
	runtime := vmoptions.Default(cfg.AppNameID, cfg.ImageMeta.Image)
	runtime.BaseDigest = "sha256:" + strings.Repeat("a", 64)
	return cfg, Layout{Runtime: runtime, Overlay: "/data/guest.qcow2", PIDFile: "/run/guest.pid", QMP: "/run/guest.qmp", Serial: "/run/guest.serial"}
}

func option(args []string, name string) string {
	for index, value := range args {
		if value == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func TestMachineBaseline(t *testing.T) {
	cfg, layout := fixture()
	if err := Validate(cfg, layout); err != nil {
		t.Fatal(err)
	}
	args := Args(cfg, layout)
	for name, want := range map[string]string{"-machine": "q35,accel=kvm", "-cpu": "host", "-smp": "2", "-m": "2048M", "-display": "gtk,gl=on"} {
		if got := option(args, name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if !slices.Contains(args, "-nodefaults") || !slices.Contains(args, "-sandbox") {
		t.Fatal("missing containment baseline")
	}
	if slices.Contains(args, "-netdev") || slices.Contains(args, "-audiodev") {
		t.Fatal("unrequested devices were granted")
	}
	if strings.Contains(strings.Join(args, " "), cfg.ImageMeta.Image) {
		t.Fatal("base attached directly instead of overlay")
	}
}

func TestReadOnlyRootIsNotSnapshot(t *testing.T) {
	cfg, layout := fixture()
	cfg.StartConditions.ReadOnlyRootfs = true
	for _, devices := range []vmoptions.Devices{vmoptions.DevicesVirtio, vmoptions.DevicesCompatible} {
		layout.Runtime.Devices = devices
		args := Args(cfg, layout)
		if !strings.Contains(option(args, "-drive"), "readonly=on") || slices.Contains(args, "-snapshot") {
			t.Fatal(args)
		}
	}
	cfg.ImageMeta.CloudInit = true
	if Validate(cfg, layout) == nil {
		t.Fatal("read-only provisioning accepted")
	}
}

func TestRawArgumentsArePhaseSpecific(t *testing.T) {
	cfg, layout := fixture()
	cfg.RunnerFlags = []string{"-append", "a b; $(touch /tmp/no)"}
	cfg.CreatorFlags = []string{"-boot", "menu=on"}
	args := Args(cfg, layout)
	if !reflect.DeepEqual(args[len(args)-2:], cfg.RunnerFlags) {
		t.Fatal(args)
	}
	layout.Installing = true
	args = Args(cfg, layout)
	if !reflect.DeepEqual(args[len(args)-2:], cfg.CreatorFlags) {
		t.Fatal(args)
	}
	if len(Warnings(cfg, layout.Runtime, true)) == 0 {
		t.Fatal("raw creator flags did not warn")
	}
}

func TestDisplayEscapesEveryShellWord(t *testing.T) {
	got := Display([]string{"qemu", "a'b", "$(false)", "", "line\nnext"})
	want := "'qemu' 'a'\\''b' '$(false)' '' 'line\nnext'"
	if got != want {
		t.Fatalf("%q != %q", got, want)
	}
}

func TestRuntimeBindingAndSizing(t *testing.T) {
	cfg, layout := fixture()
	layout.Runtime.Image = "/images/other.qcow2"
	if Validate(cfg, layout) == nil {
		t.Fatal("wrong image binding accepted")
	}
	layout.Runtime.Image = cfg.ImageMeta.Image
	cfg.ResourcesMeta.MaxCPUCores = 1.5
	if Validate(cfg, layout) == nil {
		t.Fatal("fractional VM CPU accepted")
	}
}

func TestLifecyclePoliciesAreNotSilentlyIgnored(check *testing.T) {
	cfg, layout := fixture()
	cfg.StopConditions.KeepAlive = true
	if !slices.Contains(Args(cfg, layout), "-no-shutdown") {
		check.Fatal("KeepAlive did not reach QEMU")
	}
	cfg.StopConditions.Autorestart = true
	if err := Validate(cfg, layout); err != nil {
		check.Fatal(err)
	}
	cfg.StopConditions.Autorestart = false
	cfg.StopConditions.Background = true
	if Validate(cfg, layout) == nil {
		check.Fatal("background embedded window accepted")
	}
	layout.Runtime.Display = vmoptions.DisplayNone
	if err := Validate(cfg, layout); err != nil {
		check.Fatal(err)
	}
}

func TestMediaTopologyAndSeed(t *testing.T) {
	cfg, layout := fixture()
	layout.Runtime.Devices = vmoptions.DevicesCompatible
	layout.Runtime.InstallMedia = []string{"/iso/first.iso", "/iso/second.iso", "/iso/third.iso"}
	layout.Seed = "/data/seed.iso"
	layout.Installing = true
	text := strings.Join(Args(cfg, layout), " ")
	for _, want := range []string{"ahci,id=media", "bus=media.2", "format=raw,media=cdrom,readonly=on", "bus=ahci.1", "once=d,menu=on"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in %s", want, text)
		}
	}
}

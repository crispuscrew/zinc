package validate

import (
	"math"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func baseVM() schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion, Type: schema.ZincVirtualization, AppNameID: "guest",
		ImageMeta:     schema.ImageMeta{Image: "/var/lib/zinc/images/base.qcow2"},
		ResourcesMeta: schema.ResourcesMeta{MaxRamMiB: 8192, MaxCPUCores: 4},
	}
}

func TestVMCanonicalBasePathWithoutRuntimePin(testing *testing.T) {
	if err := Validate(baseVM()); err != nil {
		testing.Fatal(err)
	}
	for _, path := range []string{"", "images/base.qcow2", "/", "/base/../image", "/base/./image", "//base/image", "/base//image", "/base/image/", "/base/a,b", "/base/a\x00b", "/base/two words"} {
		cfg := baseVM()
		cfg.ImageMeta.Image = path
		requireError(testing, cfg, "ImageMeta.Image")
	}
}

func TestVMSizing(testing *testing.T) {
	for _, cores := range []float64{0, -1, 0.5, math.NaN(), math.Inf(1)} {
		cfg := baseVM()
		cfg.ResourcesMeta.MaxCPUCores = cores
		requireError(testing, cfg, "MaxCPUCores")
	}
	for _, memory := range []int64{0, -1} {
		cfg := baseVM()
		cfg.ResourcesMeta.MaxRamMiB = memory
		requireError(testing, cfg, "MaxRamMiB")
	}
	cfg := baseVM()
	cfg.ResourcesMeta.PIDsLimit = 100
	requireError(testing, cfg, "not supported for a VM app")
}

func TestVMSharedHostPolicy(testing *testing.T) {
	cfg := baseVM()
	cfg.StartConditions = schema.StartConditions{SecureBoot: true, TPM: true, Terminal: true, DependsOn: []string{"database"}}
	cfg.StopConditions = schema.StopConditions{KeepAlive: true, Background: false, Autorestart: true}
	cfg.DisplayMeta = schema.DisplayMeta{Vulkan: true, DisplayWidth: 1920, DisplayHeight: 1080, DisableSecurityContext: true}
	cfg.ImageMeta.SourceTag = "fedora-42"
	cfg.NetworkMeta = networkCfg(outboundRule()).NetworkMeta
	cfg.AudioMeta = schema.AudioMeta{
		Playback:   schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"alsa_output.card"}, ALSADevices: []string{"/dev/snd/pcmC0D0p"}},
		Microphone: schema.AudioDevice{PipeWireDevices: []string{"alsa_input.card"}, ALSADevices: []string{"/dev/snd/pcmC0D0c"}},
		Monitor:    schema.AudioDevice{PipeWireDefault: true, PipeWireDevices: []string{"alsa_output.card"}},
	}
	if err := Validate(cfg); err != nil {
		testing.Fatal(err)
	}
	cfg.StartConditions.LoaderBIOS = true
	requireError(testing, cfg, "requires UEFI")
}

func TestVMCloudInit(testing *testing.T) {
	cfg := baseVM()
	cfg.ImageMeta.Install = []string{"dnf install -y steam"}
	cfg.ImageMeta.PublicSSHKeyPath = "/home/user/.ssh/id_ed25519.pub"
	cfg.InternalUserMeta = schema.InternalUserMeta{UseNonRootUser: true, NonRootUserName: "player"}
	requireError(testing, cfg, "ImageMeta.CloudInit")
	cfg.ImageMeta.CloudInit = true
	if err := Validate(cfg); err != nil {
		testing.Fatal(err)
	}
	for _, path := range []string{"/home/user/.ssh/id_ed25519", "relative.pub", "/home/user/../key.pub", "/home/user/key.pub\n"} {
		cfg.ImageMeta.PublicSSHKeyPath = path
		requireError(testing, cfg, "PublicSSHKeyPath")
	}
	cfg.ImageMeta.PublicSSHKeyPath = ""
	cfg.ImageMeta.Install = []string{"true\nFROM attacker"}
	requireError(testing, cfg, "control characters")
}

func TestContainerRejectsMovedVMFields(testing *testing.T) {
	for _, mutate := range []func(*schema.AppConfig){
		func(cfg *schema.AppConfig) { cfg.StartConditions.LoaderBIOS = true },
		func(cfg *schema.AppConfig) { cfg.StartConditions.SecureBoot = true },
		func(cfg *schema.AppConfig) { cfg.StartConditions.TPM = true },
		func(cfg *schema.AppConfig) { cfg.ImageMeta.CloudInit = true },
		func(cfg *schema.AppConfig) { cfg.ImageMeta.PublicSSHKeyPath = "/home/user/key.pub" },
	} {
		cfg := baseCfg()
		mutate(&cfg)
		requireError(testing, cfg, "only applies to a VM app")
	}
}

package validate

import (
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

// baseVM is a minimal valid VM app: an absolute, pinned base disk, real sizing, and an
// explicit display mode.
func baseVM() schema.AppConfig {
	return schema.AppConfig{
		SchemaVersion: schema.SchemaVersion,
		Type:          schema.ZincVirtualization,
		AppNameID:     "guest",
		ImageMeta:     schema.ImageMeta{Image: "/var/lib/zinc/images/fedora-42.qcow2"},
		VirtualizationMeta: schema.VirtualizationMeta{
			BaseDigest:  "sha256:" + strings.Repeat("a", 64),
			DiskSizeGiB: 40,
			MemoryMiB:   8192,
			VCPUs:       4,
			Display:     schema.VMDisplayAccelerated,
		},
	}
}

func TestVM_MinimalConfigIsValid(t *testing.T) {
	if err := Validate(baseVM()); err != nil {
		t.Fatalf("a minimal VM app should validate, got: %v", err)
	}
}

// The base disk is pinned by the sha256 of its bytes, the file-level equivalent of the
// rule that a third-party container image must be digest-pinned. Without it, whatever
// happens to be at that path is what boots.
func TestVM_BaseDigestRequiredAndCanonical(t *testing.T) {
	cases := []struct {
		name   string
		digest string
	}{
		{"missing", ""},
		{"no prefix", strings.Repeat("a", 64)},
		{"too short", "sha256:" + strings.Repeat("a", 63)},
		{"uppercase hex", "sha256:" + strings.Repeat("A", 64)},
		{"not hex", "sha256:" + strings.Repeat("z", 64)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseVM()
			cfg.VirtualizationMeta.BaseDigest = tc.digest
			err := Validate(cfg)
			if err == nil || !strings.Contains(err.Error(), "BaseDigest") {
				t.Fatalf("digest %q: want a BaseDigest error, got: %v", tc.digest, err)
			}
		})
	}
}

// A relative base path resolves against whatever working directory the launcher happened
// to inherit, so the same config would boot a different disk depending on where it was
// started from.
func TestVM_BasePathMustBeAbsolute(t *testing.T) {
	cfg := baseVM()
	cfg.ImageMeta.Image = "images/fedora-42.qcow2"
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("relative base path: want an absolute-path error, got: %v", err)
	}
}

// A guest has no default RAM or CPU count to fall back on, so both must be stated.
func TestVM_SizingMustBePositive(t *testing.T) {
	cfg := baseVM()
	cfg.VirtualizationMeta.MemoryMiB = 0
	cfg.VirtualizationMeta.VCPUs = 0
	err := Validate(cfg)
	if err == nil {
		t.Fatal("zero memory and vcpus: want an error, got nil")
	}
	for _, want := range []string{"MemoryMiB", "VCPUs"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %s, got: %v", want, err)
		}
	}
}

// Whether a guest gets an accelerated window is the difference between a usable game and
// an unusable one, so it is stated rather than guessed.
func TestVM_DisplayMustBeExplicitAndKnown(t *testing.T) {
	cfg := baseVM()
	cfg.VirtualizationMeta.Display = ""
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "Display") {
		t.Fatalf("empty display: want a Display error, got: %v", err)
	}
	cfg.VirtualizationMeta.Display = schema.VMDisplay("Spice")
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "Display") {
		t.Fatalf("unknown display: want a Display error, got: %v", err)
	}
	for _, mode := range []schema.VMDisplay{schema.VMDisplayNone, schema.VMDisplayWindow, schema.VMDisplayAccelerated} {
		cfg.VirtualizationMeta.Display = mode
		if err := Validate(cfg); err != nil {
			t.Errorf("display %q should be accepted, got: %v", mode, err)
		}
	}
}

// A field a VM cannot honour is an ERROR, not something quietly dropped: a config whose
// Capabilities or NetworkLists look configured but do nothing is worse than one that
// refuses to save, because the author believes in a boundary that is not there.
func TestVM_ContainerOnlyFieldsRejected(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*schema.AppConfig)
		want   string
	}{
		{"capabilities", func(cfg *schema.AppConfig) { cfg.Capabilities = []string{"CAP_NET_ADMIN"} }, "Capabilities"},
		{"network lists", func(cfg *schema.AppConfig) {
			cfg.NetworkMeta.NetworkLists = []schema.NetworkList{{Host: true}}
		}, "NetworkLists"},
		{"keys", func(cfg *schema.AppConfig) {
			cfg.Keys = []schema.Key{{Type: schema.SSH, Path: "/home/u/.ssh/id_ed25519"}}
		}, "Keys"},
		{"volumes", func(cfg *schema.AppConfig) {
			cfg.Volumes = []schema.Volume{{HostMounted: true, HostMount: "/data", InnerMount: "/data"}}
		}, "Volumes"},
		{"configs", func(cfg *schema.AppConfig) {
			cfg.Configs = []schema.Volume{{InnerMount: "/etc/app"}}
		}, "Configs"},
		{"host theme", func(cfg *schema.AppConfig) { cfg.HostTheme = true }, "HostTheme"},
		{"multiterminal", func(cfg *schema.AppConfig) {
			cfg.StartConditions.Terminal = true
			cfg.StartConditions.Multiterminal = true
			cfg.StartConditions.Entrypoint = "sh"
		}, "Multiterminal"},
		{"internal user", func(cfg *schema.AppConfig) { cfg.InternalUserMeta.UseNonRootUser = true }, "InternalUserMeta"},
		{"resources", func(cfg *schema.AppConfig) { cfg.ResourcesMeta.MaxRamMiB = 2048 }, "ResourcesMeta"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseVM()
			tc.mutate(&cfg)
			err := Validate(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s on a VM app: want an error naming %s, got: %v", tc.name, tc.want, err)
			}
			if !strings.Contains(err.Error(), "not supported for a VM app") {
				t.Errorf("the error should say the field is unsupported for a VM, got: %v", err)
			}
		})
	}
}

// The mirror of the rule above: VM fields on a container app would be inert, so they are
// rejected rather than ignored.
func TestContainer_VirtualizationFieldsRejected(t *testing.T) {
	cfg := baseCfg()
	cfg.VirtualizationMeta.MemoryMiB = 4096
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "VirtualizationMeta") {
		t.Fatalf("VM fields on a container app: want a VirtualizationMeta error, got: %v", err)
	}
}

// A container app is judged exactly as before: its image still needs a digest pin, and
// the VM rules do not leak into it.
func TestContainer_StillRequiresDigestPin(t *testing.T) {
	cfg := baseCfg()
	cfg.ImageMeta.Image = "docker.io/library/alpine:3.20"
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "digest-pinned") {
		t.Fatalf("unpinned container image: want a digest-pin error, got: %v", err)
	}
}

// A VM base is a path, not a registry reference, so it must not be held to the container
// reference rules - an absolute path has no @sha256: tail.
func TestVM_BasePathNotHeldToContainerImageRules(t *testing.T) {
	cfg := baseVM()
	if err := Validate(cfg); err != nil {
		t.Fatalf("an absolute base path with a separate digest should validate, got: %v", err)
	}
}

// zvr runs rootless and cannot bind a privileged port; catching it here beats a bind
// error at launch that says nothing about the config behind it.
func TestVM_PrivilegedHostPortRejected(t *testing.T) {
	cfg := baseVM()
	cfg.VirtualizationMeta.ForwardPorts = []schema.PortForward{{HostPort: 80, GuestPort: 80}}
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "privileged port") {
		t.Fatalf("privileged host port: want a privileged-port error, got: %v", err)
	}

	cfg.VirtualizationMeta.ForwardPorts = []schema.PortForward{{HostPort: 2222, GuestPort: 22}}
	if err := Validate(cfg); err != nil {
		t.Fatalf("unprivileged forward should validate, got: %v", err)
	}
}

func TestVM_PortRangeChecked(t *testing.T) {
	cfg := baseVM()
	cfg.VirtualizationMeta.ForwardPorts = []schema.PortForward{{HostPort: 2222, GuestPort: 70000}}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "GuestPort") {
		t.Fatalf("out-of-range guest port: want a GuestPort error, got: %v", err)
	}
}

// The seed ISO is readable by the guest, so a private key placed here would be handed to
// it. Only a .pub path is accepted.
func TestVM_PrivateSSHKeyRejected(t *testing.T) {
	cfg := baseVM()
	cfg.VirtualizationMeta.CloudInit.SSHKeyPath = "/home/user/.ssh/id_ed25519"
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "PUBLIC key") {
		t.Fatalf("private key path: want a public-key error, got: %v", err)
	}

	cfg.VirtualizationMeta.CloudInit.SSHKeyPath = "/home/user/.ssh/id_ed25519.pub"
	if err := Validate(cfg); err != nil {
		t.Fatalf("a .pub key path should validate, got: %v", err)
	}
}

// Identity fields alongside a disabled cloud-init would be written nowhere.
func TestVM_DisabledCloudInitWithIdentityRejected(t *testing.T) {
	cfg := baseVM()
	cfg.VirtualizationMeta.CloudInit = schema.CloudInit{Disabled: true, UserName: "player"}
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "CloudInit") {
		t.Fatalf("disabled cloud-init with a user name: want a CloudInit error, got: %v", err)
	}
}

func TestVM_CloudInitUserNameCharset(t *testing.T) {
	cfg := baseVM()
	cfg.VirtualizationMeta.CloudInit.UserName = "Player One"
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "UserName") {
		t.Fatalf("invalid user name: want a UserName error, got: %v", err)
	}
}

// Install steps are shared: for a VM they become cloud-init runcmd lines, so the same
// control-character rule applies to both types.
func TestVM_InstallStepsStillScreened(t *testing.T) {
	cfg := baseVM()
	cfg.VirtualizationMeta.CloudInit.UserName = "player"
	cfg.ImageMeta.Install = []string{"dnf install -y steam"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("a clean install step should validate on a VM app, got: %v", err)
	}
	cfg.ImageMeta.Install = []string{"true\nrm -rf /"}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("install step with a newline: want a control-characters error, got: %v", err)
	}
}

// The type itself must still be one of the two known values.
func TestType_UnknownRejected(t *testing.T) {
	cfg := baseVM()
	cfg.Type = schema.Type("ZincJail")
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "ZincJail") {
		t.Fatalf("unknown type: want an error naming it, got: %v", err)
	}
}

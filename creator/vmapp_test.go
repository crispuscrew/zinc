package main

import (
	"fmt"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/store"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// zc authors both app types now, so `new --vm` has to write a config the VM runtime
// accepts: pinned base, real sizing, an explicit display mode.
func TestNewVM_WritesAValidVMApp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	err := run([]string{"new", "guest", "--vm",
		"--image", "/var/lib/zinc/images/fedora.qcow2",
		"--base-digest", testDigest,
		"--memory", "8192", "--vcpus", "4", "--disk", "40",
		"--display", "Accelerated"})
	if err != nil {
		t.Fatalf("new --vm: %v", err)
	}

	sto, err := store.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := sto.Load("guest")
	if err != nil {
		t.Fatalf("load the written app: %v", err)
	}
	if cfg.Type != schema.ZincVirtualization {
		t.Errorf("Type = %q, want %q", cfg.Type, schema.ZincVirtualization)
	}
	virt := cfg.VirtualizationMeta
	if virt.BaseDigest != testDigest || virt.MemoryMiB != 8192 || virt.VCPUs != 4 || virt.DiskSizeGiB != 40 {
		t.Errorf("VirtualizationMeta = %+v, want the flags as given", virt)
	}
	if virt.Display != schema.VMDisplayAccelerated {
		t.Errorf("Display = %q, want %q", virt.Display, schema.VMDisplayAccelerated)
	}
}

// A VM app without its pin must not save: the digest is what makes the base image the one
// that was authorised, and Save runs the same validation the runtime does.
func TestNewVM_RequiresThePin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	err := run([]string{"new", "guest", "--vm", "--image", "/var/lib/zinc/images/fedora.qcow2"})
	if err == nil || !strings.Contains(err.Error(), "BaseDigest") {
		t.Fatalf("a VM app with no base digest should be refused, got: %v", err)
	}
}

// VM flags on a container app would be written into a config where they do nothing, which
// is the trap the cross-type validation exists to close - so the CLI refuses them early
// with a message about the flag the author actually forgot.
func TestNew_VMFlagsWithoutVMRefused(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	err := run([]string{"new", "app", "--image", "localhost/app:local", "--memory", "2048"})
	if err == nil || !strings.Contains(err.Error(), "--vm") {
		t.Fatalf("VM flags without --vm should be refused, got: %v", err)
	}
}

// A container app still authors exactly as before.
func TestNew_ContainerUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	if err := run([]string{"new", "app", "--image", "localhost/app:local"}); err != nil {
		t.Fatalf("new (container): %v", err)
	}
	sto, _ := store.Default()
	cfg, err := sto.Load("app")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Type != schema.ZincContainer {
		t.Errorf("Type = %q, want %q", cfg.Type, schema.ZincContainer)
	}
	if !cfg.VirtualizationMeta.IsZero() {
		t.Errorf("a container app should carry no VM fields, got %+v", cfg.VirtualizationMeta)
	}
}

// Commands with no counterpart on the VM side are refused by name. Forwarding them to zvr
// would fail with an unknown-command error that says nothing about why, and silently
// doing nothing would be worse still.
func TestDelegateVM_UnsupportedCommandsExplainThemselves(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{"build", "no image to build"},
		{"logs", "no container log"},
		{"restart", "zc stop"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			err := delegateVM(tc.command, "guest", []string{"guest"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s on a VM app: want an error containing %q, got: %v", tc.command, tc.want, err)
			}
			if !strings.Contains(err.Error(), "VM app") {
				t.Errorf("the error should say the app is a VM, got: %v", err)
			}
		})
	}
}

// zc's own contract is that a bare `run` shows the plan and `--exec` performs it. zvr's
// default is the opposite, so the flag has to be translated rather than forwarded.
func TestDelegateVM_RunTranslatesTheExecContract(t *testing.T) {
	// The mapping is asserted through the argument helpers the translation is built from,
	// since running it would need a zvr binary on PATH.
	if got := firstPositional([]string{"--exec", "guest"}); got != "guest" {
		t.Errorf("firstPositional = %q, want the app name regardless of flag order", got)
	}
	if !hasFlag([]string{"guest", "--exec"}, "--exec") {
		t.Error("--exec should be recognised after the name")
	}
	if hasFlag([]string{"guest"}, "--exec") {
		t.Error("a bare run must not look like --exec")
	}
	if got := flagsOnly([]string{"guest", "--force"}); len(got) != 1 || got[0] != "--force" {
		t.Errorf("flagsOnly = %v, want just the flags", got)
	}
}

// An argument that is not a defined app is forwarded rather than judged: it may be a raw
// container name or a path the container runtime understands and the store does not.
func TestDelegate_UnknownAppFallsThroughToTheContainerRuntime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("PATH", "") // no runtime installed, so the forward fails in a recognisable way
	quiet(t)

	sto, err := store.Default()
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Dir(sto.Path("x")), 0o755)

	err = delegate(backend.New(sto), "stop", []string{"not-an-app"})
	if err == nil || !strings.Contains(err.Error(), "zcr") {
		t.Fatalf("an unknown name should be forwarded to the container runtime, got: %v", err)
	}
}

// --resolution is one flag rather than two because a width without a height is not a screen.
func TestParseResolution(t *testing.T) {
	cases := []struct {
		spec          string
		width, height int
		wantErr       string
	}{
		{"", 0, 0, ""},
		{"   ", 0, 0, ""},
		{"1920x1080", 1920, 1080, ""},
		{"3840X2160", 3840, 2160, ""}, // the separator is case-insensitive
		{" 1280 x 800 ", 1280, 800, ""},
		{"1920", 0, 0, "want WxH"},
		{"1920x1080x60", 0, 0, "want WxH"},
		{"widexhigh", 0, 0, "not a number"},
		{"1920xtall", 0, 0, "height"},
	}
	for _, tc := range cases {
		width, height, err := parseResolution(tc.spec)
		if tc.wantErr != "" {
			if err == nil {
				t.Errorf("%q should be refused", tc.spec)
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%q: error %q should mention %q", tc.spec, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.spec, err)
		}
		if width != tc.width || height != tc.height {
			t.Errorf("%q = %dx%d, want %dx%d", tc.spec, width, height, tc.width, tc.height)
		}
	}
}

// --mac random is drawn once, here, and the literal address is stored. A config holding the
// word "random" would draw a new address on every run, and a guest whose NIC changes
// underneath it loses its DHCP lease and looks to Windows like swapped hardware.
func TestResolveMac_RandomIsDrawnOnceAndLocallyAdministered(t *testing.T) {
	if got, err := resolveMac(""); err != nil || got != "" {
		t.Errorf("resolveMac(\"\") = %q, %v; want the derived default left alone", got, err)
	}
	if got, err := resolveMac("02:1a:2b:3c:4d:5e"); err != nil || got != "02:1a:2b:3c:4d:5e" {
		t.Errorf("an explicit address should pass through, got %q, %v", got, err)
	}

	seen := map[string]bool{}
	for range 32 {
		mac, err := resolveMac("RANDOM") // the keyword is case-insensitive
		if err != nil {
			t.Fatal(err)
		}
		if seen[mac] {
			t.Fatalf("%q was drawn twice; the address is not actually random", mac)
		}
		seen[mac] = true

		var first uint64
		if _, err := fmt.Sscanf(mac[:2], "%02x", &first); err != nil {
			t.Fatalf("%q does not start with a hex octet", mac)
		}
		// Locally administered (bit 1 set): belongs to no vendor, so unlike the default it
		// identifies nothing. Unicast (bit 0 clear): a multicast NIC cannot hold a conversation.
		if first&0x02 == 0 {
			t.Errorf("%q is not locally administered, so it claims a vendor's address space", mac)
		}
		if first&0x01 != 0 {
			t.Errorf("%q is multicast; a NIC needs a unicast address", mac)
		}
		if strings.HasPrefix(mac, "52:54:00") {
			t.Errorf("%q kept QEMU's prefix, which is the thing a random address exists to avoid", mac)
		}
	}
}

// A randomly drawn address still has to survive the validation that screens a hand-written
// one, or --mac random would author configs that refuse to save.
func TestResolveMac_RandomPassesValidation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	for index := range 8 {
		name := fmt.Sprintf("guest%d", index)
		if err := run([]string{"new", name, "--vm",
			"--image", "/var/lib/zinc/images/fedora.qcow2",
			"--base-digest", testDigest,
			"--memory", "4096", "--vcpus", "2",
			"--display", "Accelerated", "--mac", "random"}); err != nil {
			t.Fatalf("new --vm --mac random: %v", err)
		}
	}
}

// --media is how a guest is handed a disc it will need after the OS is on it: virtio-win's
// display driver, a tools ISO. A trailing comma is a typo, not a path, so it does not become
// an empty entry validation would then reject.
func TestNewVM_MediaDiscs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	quiet(t)

	if err := run([]string{"new", "guest", "--vm",
		"--image", "/var/lib/zinc/images/windows.qcow2",
		"--base-digest", testDigest,
		"--memory", "4096", "--vcpus", "2", "--display", "Compatible",
		"--media", "/iso/virtio-win.iso, /iso/tools.iso,"}); err != nil {
		t.Fatalf("new --vm --media: %v", err)
	}

	sto, err := store.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := sto.Load("guest")
	if err != nil {
		t.Fatalf("load the written app: %v", err)
	}
	got := cfg.VirtualizationMeta.InstallMedia
	if len(got) != 2 || got[0] != "/iso/virtio-win.iso" || got[1] != "/iso/tools.iso" {
		t.Errorf("InstallMedia = %q, want both discs trimmed and the empty entry dropped", got)
	}
}

// --tunnel must author an app that WORKS, not just one that has the field set. The tunnel is
// built inside a namespace whose ruleset default-drops, so without a rule permitting UDP to
// the peer's endpoint the handshake never leaves and the interface carries nothing.
func TestNewTunnel_SeedsTheEndpointEgressRule(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "wg.conf")
	if err := os.WriteFile(conf, []byte("[Interface]\nPrivateKey = k\nAddress = 10.9.0.2/24\n"+
		"[Peer]\nPublicKey = p\nEndpoint = 203.0.113.7:51820\nAllowedIPs = 0.0.0.0/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var cfg schema.AppConfig
	if err := seedTunnel(&cfg, conf); err != nil {
		t.Fatal(err)
	}
	if cfg.NetworkMeta.Tunnel.WireGuardConf != conf {
		t.Errorf("WireGuardConf = %q, want the absolute path %q", cfg.NetworkMeta.Tunnel.WireGuardConf, conf)
	}
	if len(cfg.NetworkMeta.NetworkLists) != 1 {
		t.Fatalf("want one seeded egress list, got %+v", cfg.NetworkMeta.NetworkLists)
	}
	list := cfg.NetworkMeta.NetworkLists[0]
	if len(list.IPv4CIDR) != 1 || list.IPv4CIDR[0] != "203.0.113.7/32" {
		t.Errorf("the endpoint address should be allowed, got %v", list.IPv4CIDR)
	}
	if len(list.Ports) != 1 || list.Ports[0] != 51820 {
		t.Errorf("the endpoint PORT should be allowed, got %v", list.Ports)
	}
	// And the whole thing must actually pass validation, which is the point of seeding it.
	cfg.SchemaVersion, cfg.Type, cfg.AppNameID = schema.SchemaVersion, schema.ZincContainer, "vpn"
	cfg.ImageMeta.Image = "localhost/vpn:local"
	if err := validate.Validate(cfg); err != nil {
		t.Fatalf("a --tunnel app should be valid as authored, got: %v", err)
	}
}

// A relative path is made absolute: the runner reads it from wherever it happens to be
// running, and a relative one would name a different file depending on the caller.
func TestNewTunnel_MakesThePathAbsolute(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wg.conf"), []byte("[Interface]\nPrivateKey = k\nAddress = 10.9.0.2/24\n"+
		"[Peer]\nPublicKey = p\nEndpoint = 203.0.113.7:51820\nAllowedIPs = 0.0.0.0/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var cfg schema.AppConfig
	if err := seedTunnel(&cfg, "wg.conf"); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg.NetworkMeta.Tunnel.WireGuardConf) {
		t.Errorf("WireGuardConf = %q, want an absolute path", cfg.NetworkMeta.Tunnel.WireGuardConf)
	}
}

// A config Zinc cannot apply is refused at authoring time, where it is cheapest to fix,
// rather than at the first launch.
func TestNewTunnel_RefusesAConfigItCannotApply(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "wg.conf")
	if err := os.WriteFile(conf, []byte("[Interface]\nPrivateKey = k\nAddress = 10.9.0.2/24\nPostUp = rm -rf /\n"+
		"[Peer]\nPublicKey = p\nEndpoint = 203.0.113.7:51820\nAllowedIPs = 0.0.0.0/0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var cfg schema.AppConfig
	if err := seedTunnel(&cfg, conf); err == nil {
		t.Fatal("a config with a script hook must be refused")
	}
	if err := seedTunnel(&cfg, filepath.Join(dir, "absent.conf")); err == nil {
		t.Fatal("a missing config must be refused")
	}
}

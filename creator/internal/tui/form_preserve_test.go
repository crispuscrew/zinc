package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func richConfig() schema.AppConfig {
	cfg := sample("app")
	cfg.LauncherMeta = schema.LauncherMeta{Description: "description", Icon: "/icons/a b.png", Group: "Work"}
	cfg.StartConditions = schema.StartConditions{Entrypoint: " run ", EntrypointEnv: map[string]string{"TOKEN": "a=b c"}, AttachedEnv: map[string]string{"SESSION": "private"}, AttachedEntrypoint: "shell", Attached: true, Terminal: true, ReadOnlyRootfs: true}
	cfg.StopConditions = schema.StopConditions{KeepAlive: true, Background: true, Autorestart: true}
	cfg.MinimizeFingerprint, cfg.HostTheme = true, true
	cfg.ResourcesMeta = schema.ResourcesMeta{MaxCPUCores: 0.5, MaxRamMiB: 1024, PIDsLimit: 100}
	cfg.ImageMeta.Install = []string{" line with whitespace "}
	cfg.ImageMeta.SourceTag = "registry/image:tag"
	cfg.InternalUserMeta = schema.InternalUserMeta{UseNonRootUser: true, NonRootUserName: "user", KeepUserID: true}
	cfg.DisplayMeta = schema.DisplayMeta{RequireSecurityContext: true, DisplayWidth: 1920, DisplayHeight: 1080, Vulkan: true}
	cfg.AudioMeta.Playback = schema.AudioDevice{PipeWireDevices: []string{"an exact sink name"}}
	cfg.AudioMeta.Microphone = schema.AudioDevice{ALSADevices: []string{"/dev/snd/pcmC0D0c"}}
	cfg.AudioMeta.Monitor = schema.AudioDevice{PipeWireDefault: true}
	cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "primary", MacAddress: "02:11:22:33:44:55"}}
	cfg.NetworkMeta.RulesByPriority = []schema.NetworkRule{{From: schema.NetworkPeer{Type: schema.NetworkPeerSelf}, To: schema.NetworkPeer{Type: schema.NetworkPeerInternet}, Domains: []string{"example.com"}, Protocols: []schema.NetworkProtocol{schema.NetworkTCP}}}
	cfg.NetworkMeta.DNS.ResolversByPriority = []schema.DNSResolver{{Protocol: schema.DNSHTTPS, Endpoint: "dns.example", Path: "/dns-query", BootstrapIPs: []string{"1.1.1.1"}}}
	cfg.DBusMeta = schema.DBusMeta{Talk: []string{"org.example.App"}, Own: []string{"org.example.Owner"}}
	cfg.NotificationMeta = schema.NotificationMeta{Silenced: true, AllowedLinks: true}
	cfg.Configs = []schema.ConfigFile{{BundlePath: "app.ini", InnerMount: "/app.ini"}}
	cfg.Volumes = []schema.Volume{{InnerMount: "/scratch", Writable: true, SizeLimited: true, SizeLimitMiB: 100}}
	cfg.Keys = []schema.Key{{Type: schema.SSH, Path: "/keys/id"}}
	cfg.CreatorFlags = []string{"--label", "literal $(not-a-shell) with spaces"}
	cfg.RunnerFlags = []string{"--env", "LONG=" + strings.Repeat("x", 2048)}
	return cfg
}

func TestFormNoOpPreservesEveryAuthoredSelection(t *testing.T) {
	expected := richConfig()
	frm := newForm(expected, false)
	actual := frm.toConfig()
	if frm.err != nil {
		t.Fatal(frm.err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("no-op edit lost data:\n%+v\n%+v", actual, expected)
	}
}

func TestEditorReloadRebindsAllControls(t *testing.T) {
	frm := newForm(sample("app"), false)
	expected := richConfig()
	frm.reload(expected)
	actual := frm.toConfig()
	if frm.err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("reload overwrote an editor value: %+v; %v", actual, frm.err)
	}
}

func TestStructuredEditsDoNotMutateOriginalOrOtherDirections(t *testing.T) {
	base := richConfig()
	frm := newForm(base, false)
	frm.fields[fieldIdx(frm, "audio.playback.PipeWireDevices")].area.SetValue("[]")
	frm.fields[fieldIdx(frm, "attached env (YAML map)")].area.SetValue("{SESSION: changed}")
	frm.fields[fieldIdx(frm, "runner flags (argv array)")].area.SetValue("['--label', 'two words', '$(literal)']")
	actual := frm.toConfig()
	if frm.err != nil {
		t.Fatal(frm.err)
	}
	if !actual.AudioMeta.Playback.IsZero() || actual.AudioMeta.Microphone.ALSADevices[0] != "/dev/snd/pcmC0D0c" || actual.StartConditions.AttachedEnv["SESSION"] != "changed" {
		t.Fatal(actual)
	}
	if len(actual.RunnerFlags) != 3 || actual.RunnerFlags[1] != "two words" || actual.RunnerFlags[2] != "$(literal)" {
		t.Fatal(actual.RunnerFlags)
	}
	if base.StartConditions.AttachedEnv["SESSION"] != "private" || len(base.AudioMeta.Playback.PipeWireDevices) != 1 {
		t.Fatal("working draft mutated source")
	}
}

func TestMalformedStructuredInputBlocksSave(t *testing.T) {
	frm := newForm(sample("app"), false)
	frm.fields[fieldIdx(frm, "runner flags (argv array)")].area.SetValue("{not: an-array}")
	frm.toConfig()
	if frm.err == nil {
		t.Fatal("malformed argv silently saved")
	}
}

func TestWarningAndFocusedFieldStayVisible(t *testing.T) {
	frm := newForm(richConfig(), false)
	frm.height = 24
	frm.focus(fieldIdx(frm, "runner flags (argv array)"))
	view := frm.view()
	if !strings.Contains(view, "runner flags") || !strings.Contains(view, "WARNING: raw backend") {
		t.Fatal(view)
	}
}

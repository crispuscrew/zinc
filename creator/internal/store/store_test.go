package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
)

const digestPin = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func sampleApp(name string) schema.AppConfig {
	return schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincContainer, AppNameID: name, ImageMeta: schema.ImageMeta{Image: "docker.io/library/" + name + digestPin}}
}

func tempStore(t *testing.T) *Store { t.Helper(); return &Store{Root: t.TempDir()} }

func TestSaveLoadRoundTrip(t *testing.T) {
	sto := tempStore(t)
	expected := sampleApp("firefox")
	if err := sto.Save(expected); err != nil {
		t.Fatal(err)
	}
	actual, err := sto.Load("firefox")
	if err != nil {
		t.Fatal(err)
	}
	if actual.AppNameID != expected.AppNameID || actual.ImageMeta.Image != expected.ImageMeta.Image || actual.Type != expected.Type {
		t.Fatal(actual)
	}
	if err := validate.Validate(actual); err != nil {
		t.Fatal(err)
	}
}

func TestListExistsDelete(t *testing.T) {
	sto := tempStore(t)
	if names, err := sto.List(); err != nil || len(names) != 0 {
		t.Fatal(names, err)
	}
	if sto.Exists("firefox") {
		t.Fatal("missing app exists")
	}
	for _, name := range []string{"zed", "firefox", "ghostty"} {
		if err := sto.Save(sampleApp(name)); err != nil {
			t.Fatal(err)
		}
	}
	names, err := sto.List()
	if err != nil || !reflect.DeepEqual(names, []string{"firefox", "ghostty", "zed"}) {
		t.Fatal(names, err)
	}
	if !sto.Exists("firefox") {
		t.Fatal("saved app missing")
	}
	if err := sto.Delete("firefox"); err != nil {
		t.Fatal(err)
	}
	if sto.Exists("firefox") {
		t.Fatal("deleted app exists")
	}
	if err := sto.Delete("firefox"); err != nil {
		t.Fatal("delete is not idempotent", err)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	sto := tempStore(t)
	cfg := sampleApp("firefox")
	cfg.ImageMeta.Image = "alpine:latest"
	if err := sto.Save(cfg); err == nil {
		t.Fatal("unvalidated save")
	}
	if sto.Exists("firefox") {
		t.Fatal("invalid save wrote a file")
	}
}

func TestMarshalLoadRoundtrip(t *testing.T) {
	cfg := sampleApp("roundtrip")
	cfg.NetworkMeta = schema.NetworkMeta{Interfaces: []schema.NetworkInterface{{ID: "primary"}}, RulesByPriority: []schema.NetworkRule{{From: schema.NetworkPeer{Type: schema.NetworkPeerSelf}, To: schema.NetworkPeer{Type: schema.NetworkPeerInternet, Filter: schema.NetworkPeerFilter{IPv4CIDR: []string{"1.1.1.1/32"}, Ports: []int{443}}}, Protocols: []schema.NetworkProtocol{schema.NetworkTCP}}}}
	cfg.StartConditions.EntrypointEnv = map[string]string{"NAME": "value with spaces"}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "roundtrip.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	actual, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual.NetworkMeta.RulesByPriority) != 1 {
		t.Fatal("roundtrip lost network rule")
	}
	rule := actual.NetworkMeta.RulesByPriority[0]
	if rule.From.Type != schema.NetworkPeerSelf || rule.To.Type != schema.NetworkPeerInternet ||
		!reflect.DeepEqual(rule.To.Filter.IPv4CIDR, []string{"1.1.1.1/32"}) || !reflect.DeepEqual(rule.To.Filter.Ports, []int{443}) ||
		!reflect.DeepEqual(rule.Protocols, []schema.NetworkProtocol{schema.NetworkTCP}) || !reflect.DeepEqual(actual.StartConditions.EntrypointEnv, cfg.StartConditions.EntrypointEnv) {
		t.Fatal("roundtrip lost policy", actual)
	}
}

func TestLoadStrictAndEmpty(t *testing.T) {
	for _, text := range []string{"", "SchemaVersion: 4\ntyppo: drift\n", "SchemaVersion: 4\nAudioMeta:\n  Playback:\n    Unknown: true\n", "SchemaVersion: 4\n---\nAppNameID: extra\n"} {
		if _, err := decode([]byte(text), "test"); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}

func TestDefaultPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	sto, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if sto.Root != "/xdg/zinc/apps" || sto.Path("firefox") != "/xdg/zinc/apps/firefox.yaml" || sto.VMPath("firefox") != "/xdg/zinc/runtime/vm/firefox.json" {
		t.Fatal(sto)
	}
}

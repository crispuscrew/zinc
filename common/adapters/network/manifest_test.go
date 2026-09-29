package network

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/crispuscrew/zinc/common/domain/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
)

func manifestFixture() (schema.AppConfig, Manifest) {
	cfg := schema.AppConfig{AppNameID: "client", Type: schema.ZincContainer, NetworkMeta: schema.NetworkMeta{Interfaces: []schema.NetworkInterface{{ID: "main"}}}}
	manifest := Manifest{Version: 1, AppNameID: "client", Generation: "launch-1", NetworkNamespace: "/run/user/1000/client.net", UserNamespace: "/run/user/1000/client.user", NetworkInode: 123, UserInode: 456,
		PacketPreserving: true, Exclusive: true, StaticNeighbors: true, CompleteInventory: true, Policy: cfg.NetworkMeta,
		Topology: domain.Topology{Mode: domain.Container, Interfaces: []domain.Attachment{{InterfaceID: "main", Device: "eth0", MAC: "02:00:00:00:00:01", Addresses: []string{"10.0.0.2"}}}},
	}
	return cfg, manifest
}

func TestStrictManifestDecoding(t *testing.T) {
	cfg, manifest := manifestFixture()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(cfg, decoded, nil); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{"version":1,"unknown":true}`, `{"version":1,"version":1}`, `{"version":1,"Version":1}`, `{"policy":{"Interfaces":[],"Interfaces":[]}}`, `{} {}`, `null`, `[]`} {
		value, err := Decode([]byte(invalid))
		if err == nil {
			if _, err = Resolve(cfg, value, nil); err == nil {
				t.Errorf("accepted %s", invalid)
			}
		}
	}
}

func TestManifestRequiresExactTopologyAndPolicy(t *testing.T) {
	mutations := []func(*Manifest){
		func(value *Manifest) { value.PacketPreserving = false },
		func(value *Manifest) { value.Exclusive = false },
		func(value *Manifest) { value.StaticNeighbors = false },
		func(value *Manifest) { value.CompleteInventory = false },
		func(value *Manifest) { value.NetworkInode = 0 },
		func(value *Manifest) { value.NetworkNamespace = "relative" },
		func(value *Manifest) { value.AppNameID = "other" },
		func(value *Manifest) { value.Topology.Mode = domain.VirtualMachine },
		func(value *Manifest) { value.Topology.Interfaces = nil },
		func(value *Manifest) { value.Policy.RulesByPriority = []schema.NetworkRule{{}} },
	}
	for index, mutate := range mutations {
		cfg, manifest := manifestFixture()
		mutate(&manifest)
		if _, err := Resolve(cfg, manifest, nil); err == nil {
			t.Errorf("invalid manifest case %d", index)
		}
	}
}

func TestSecureLoadingRejectsSymlinksAndWritableFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(link); err == nil {
		t.Fatal("followed manifest symlink")
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("accepted writable manifest")
	}
	if _, err := Load(schema.AppConfig{AppNameID: "../escape"}); err == nil {
		t.Fatal("accepted app path escape")
	}
}

func TestMissingManifestIsActionable(t *testing.T) {
	t.Setenv("ZINC_NETWORK_MANIFEST_DIR", t.TempDir())
	_, err := Load(schema.AppConfig{AppNameID: "missing"})
	if err == nil || !strings.Contains(err.Error(), "preprovisioned packet-preserving topology") {
		t.Fatalf("bad missing topology error: %v", err)
	}
}

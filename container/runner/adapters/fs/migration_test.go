package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeRejectsUnknownAndUnrepresentableLegacy(t *testing.T) {
	for _, residue := range []string{
		"typpo: drift", "Capabilities: [NET_ADMIN]", "ResourcesMeta: {MaxSwapMiB: 32}",
		"StartConditions: {ReadyCheck: [true]}", "StartConditions: {ReadyTimeoutSec: 3}",
		"NetworkMeta: {Tunnel: {WireGuardConf: /vpn.conf}}",
	} {
		for _, version := range []string{"3", "4"} {
			body := "SchemaVersion: " + version + "\nType: ZincContainer\nAppNameID: demo\nImageMeta: {Image: localhost/demo:local}\n" + residue + "\n"
			if _, err := decode([]byte(body), "legacy.yaml"); err == nil {
				t.Errorf("v%s silently accepted %s", version, residue)
			}
		}
	}
}

func TestLegacyRenamesAndZeroResidue(t *testing.T) {
	body := `SchemaVersion: 3
Type: ZincContainer
AppNameID: demo
Description: old description
Env: {BASE: keep}
ReadOnlyRootfs: true
Capabilities: []
ResourcesMeta: {MaxSwapMiB: 0}
StartConditions:
  Terminal: true
  Multiterminal: true
  MultiterminalEntrypoint: echo hello
  MultiterminalEnv: {BASE: overlay}
  Autorestart: true
  ReadyCheck: []
  ReadyTimeoutSec: 0
`
	cfg, err := decode([]byte(body), "legacy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LauncherMeta.Description != "old description" || !cfg.StartConditions.Attached || !cfg.StartConditions.ReadOnlyRootfs || !cfg.StopConditions.Autorestart {
		t.Fatalf("lost migrated values: %+v", cfg)
	}
	if cfg.StartConditions.EntrypointEnv["BASE"] != "keep" || cfg.StartConditions.AttachedEnv["BASE"] != "overlay" {
		t.Fatal(cfg.StartConditions)
	}
}

func TestResolvedEnvironmentMapsReplaceInheritedMaps(t *testing.T) {
	store := tempStore(t)
	parent := "SchemaVersion: 4\nType: ZincContainer\nAppNameID: parent\nStartConditions:\n  EntrypointEnv: {KEEP: parent, DROP: parent}\n  AttachedEnv: {DROP: parent}\n"
	child := "SchemaVersion: 4\nType: ZincContainer\nAppNameID: child\nInherits: parent\nStartConditions:\n  EntrypointEnv: {ONLY: child}\n  AttachedEnv: {}\n"
	for name, body := range map[string]string{"parent": parent, "child": child} {
		if err := os.WriteFile(filepath.Join(store.Root, name+".yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := store.LoadResolved("child")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.StartConditions.EntrypointEnv) != 1 || cfg.StartConditions.EntrypointEnv["ONLY"] != "child" || len(cfg.StartConditions.AttachedEnv) != 0 {
		t.Fatal(cfg.StartConditions)
	}
	data, err := os.ReadFile(store.Path("child"))
	if err != nil || !strings.Contains(string(data), "Inherits: parent") {
		t.Fatalf("load rewrote inheritance: %s %v", data, err)
	}
}

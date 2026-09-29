package disk

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/vmoptions"
	"gopkg.in/yaml.v3"
)

func vmCfg() schema.AppConfig {
	return schema.AppConfig{SchemaVersion: schema.SchemaVersion, Type: schema.ZincVirtualization, AppNameID: "guest",
		ImageMeta: schema.ImageMeta{CloudInit: true}, InternalUserMeta: schema.InternalUserMeta{UseNonRootUser: true, NonRootUserName: "player"}}
}

func TestUserDataIdentityWithoutImplicitPrivilege(t *testing.T) {
	document, err := userData(vmCfg())
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(document), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["hostname"] != "guest" || parsed["ssh_pwauth"] != false {
		t.Fatal(parsed)
	}
	account := parsed["users"].([]any)[0].(map[string]any)
	if account["name"] != "player" || account["sudo"] != false || account["lock_passwd"] != true {
		t.Fatal(account)
	}
}

func TestInstallCommandsRemainData(t *testing.T) {
	cfg := vmCfg()
	cfg.ImageMeta.Install = []string{"echo it's fine", "true\nnot-a-key: value"}
	document, err := userData(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Commands []string `yaml:"runcmd"`
	}
	if err := yaml.Unmarshal([]byte(document), &parsed); err != nil {
		t.Fatal(err)
	}
	if strings.Join(parsed.Commands, "|") != strings.Join(cfg.ImageMeta.Install, "|") {
		t.Fatalf("commands changed: %q", document)
	}
}

func TestPublicAndPrivateKeys(t *testing.T) {
	cfg := vmCfg()
	cfg.ImageMeta.PublicSSHKeyPath = filepath.Join(t.TempDir(), "key.pub")
	if err := os.WriteFile(cfg.ImageMeta.PublicSSHKeyPath, []byte("-----BEGIN OPENSSH PRIVATE KEY-----"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := userData(cfg); err == nil || !strings.Contains(err.Error(), "PRIVATE key") {
		t.Fatalf("got %v", err)
	}
	blob := make([]byte, 4+11+4+32)
	binary.BigEndian.PutUint32(blob, 11)
	copy(blob[4:], "ssh-ed25519")
	binary.BigEndian.PutUint32(blob[15:], 32)
	key := "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " test"
	if err := os.WriteFile(cfg.ImageMeta.PublicSSHKeyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := userData(cfg)
	if err != nil || !strings.Contains(document, key) {
		t.Fatalf("%s %v", document, err)
	}
	metadata, err := metaData(cfg)
	if err != nil || !strings.Contains(metadata, key) {
		t.Fatalf("%s %v", metadata, err)
	}
}

func TestCloudInitMustBeExplicit(t *testing.T) {
	cfg := vmCfg()
	cfg.ImageMeta.CloudInit = false
	cfg.ImageMeta.PublicSSHKeyPath = "/missing/key.pub"
	files, err := seedFiles(cfg, vmoptions.DevicesCompatible)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files["zinc-setup.cmd"] == "" {
		t.Fatal(files)
	}
	files, err = seedFiles(cfg, vmoptions.DevicesVirtio)
	if err != nil || len(files) != 0 {
		t.Fatalf("%v %v", files, err)
	}
}

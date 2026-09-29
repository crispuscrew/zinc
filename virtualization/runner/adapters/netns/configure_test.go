package netns

import (
	"testing"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
)

func TestConfigureUsesProvisionedMACWithoutMutatingAuthoring(t *testing.T) {
	cfg, manifest, argv := vmFixture()
	cfg.NetworkMeta.Interfaces[0].MacAddress = ""
	manifest.Policy = cfg.NetworkMeta
	bound, attachments, err := ConfigureResolved(cfg, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NetworkMeta.Interfaces[0].MacAddress != "" || bound.NetworkMeta.Interfaces[0].MacAddress != manifest.Topology.Interfaces[0].MAC || len(attachments) != 1 || attachments[0].TapName != "ztap0" {
		t.Fatal("runtime binding mutated original or lost identity")
	}
	if _, _, err := CommandResolved(bound, argv, "", manifest, nil); err != nil {
		t.Fatal("bound config no longer matches manifest", err)
	}
	manifest.Topology.Interfaces[0].InterfaceID = "other"
	if _, _, err := ConfigureResolved(cfg, manifest); err == nil {
		t.Fatal("accepted missing interface")
	}
}

func TestConfigureRejectsWrongAppAndPolicy(t *testing.T) {
	cfg, manifest, _ := vmFixture()
	manifest.AppNameID = "other"
	if _, _, err := ConfigureResolved(cfg, manifest); err == nil {
		t.Fatal("cross-app manifest accepted")
	}
	if _, _, err := ConfigureResolved(cfg, provision.Manifest{}); err == nil {
		t.Fatal("empty manifest accepted")
	}
}

package disk

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"gopkg.in/yaml.v3"
)

func metaData(cfg schema.AppConfig) (string, error) {
	document := map[string]any{"instance-id": "zinc-" + cfg.AppNameID, "local-hostname": cfg.AppNameID}
	if cfg.ImageMeta.PublicSSHKeyPath != "" {
		key, err := publicKey(cfg.ImageMeta.PublicSSHKeyPath)
		if err != nil {
			return "", err
		}
		document["public-keys"] = []string{key}
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	return string(encoded) + "\n", err
}

func userData(cfg schema.AppConfig) (string, error) {
	return userDataWithIDs(cfg, os.Getuid(), os.Getgid())
}

func userDataWithIDs(cfg schema.AppConfig, userID, groupID int) (string, error) {
	document := map[string]any{"hostname": cfg.AppNameID, "ssh_pwauth": false}
	var authorized []string
	if cfg.ImageMeta.PublicSSHKeyPath != "" {
		key, err := publicKey(cfg.ImageMeta.PublicSSHKeyPath)
		if err != nil {
			return "", err
		}
		authorized = []string{key}
	}
	user := cfg.InternalUserMeta
	if user.UseNonRootUser {
		if user.NonRootUserName == "" || user.NonRootUserName == "root" {
			return "", fmt.Errorf("UseNonRootUser requires a non-root NonRootUserName")
		}
		account := map[string]any{"name": user.NonRootUserName, "lock_passwd": true, "shell": "/bin/sh", "sudo": false}
		if len(authorized) > 0 {
			account["ssh_authorized_keys"] = authorized
		}
		if user.KeepUserID {
			if userID <= 0 || groupID <= 0 {
				return "", fmt.Errorf("KeepUserID with UseNonRootUser requires nonzero host UID and GID")
			}
			// cloud-init supports user uid but not a numeric group creation mapping.
			// Do not claim full identity preservation until both can be enforced.
			return "", fmt.Errorf("InternalUserMeta.KeepUserID: VM UID/GID preservation requires a guest identity provisioner")
		}
		document["users"] = []any{account}
	} else if len(authorized) > 0 {
		document["ssh_authorized_keys"] = authorized
	}
	if len(cfg.ImageMeta.Install) > 0 {
		document["runcmd"] = cfg.ImageMeta.Install
	}
	encoded, err := yaml.Marshal(document)
	return "#cloud-config\n" + string(encoded), err
}

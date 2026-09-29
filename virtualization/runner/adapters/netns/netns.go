// Package netns enforces VM policy on actual TAP packets in a provisioned netns.
package netns

import (
	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/nftrules"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/qemu"
)

const Binary = "nsenter"

func Applies(cfg schema.AppConfig) bool { return len(cfg.NetworkMeta.Interfaces) > 0 }

// Attachments must populate app.Service.NetworkAttachments before qemu.Args.
// The provisioner owns addressing, static neighbors and host publication.
func Attachments(cfg schema.AppConfig) ([]qemu.NetworkAttachment, error) {
	if !Applies(cfg) {
		return nil, nil
	}
	manifest, err := provision.Load(cfg)
	if err != nil {
		return nil, err
	}
	_, result, err := ConfigureResolved(cfg, manifest)
	return result, err
}

func Command(cfg schema.AppConfig, argv []string, resolvPath string) ([]string, string, error) {
	if !Applies(cfg) {
		return isolatedCommand(argv)
	}
	manifest, err := provision.Load(cfg)
	if err != nil {
		return nil, "", err
	}
	return CommandResolved(cfg, argv, resolvPath, manifest, nil)
}

// CommandResolved is the DNS worker's integration seam. Lookup is explicitly
// configured; failure to resolve either endpoint's domain policy aborts launch.
func CommandResolved(cfg schema.AppConfig, argv []string, _ string, manifest provision.Manifest, lookup provision.Lookup) ([]string, string, error) {
	plan, err := provision.Resolve(cfg, manifest, lookup)
	if err != nil {
		return nil, "", err
	}
	if err := provision.ReadyDNS(cfg, manifest); err != nil {
		return nil, "", err
	}
	if err := checkAttachments(cfg, argv, manifest); err != nil {
		return nil, "", err
	}
	ruleset, err := nftrules.RenderResolved(plan.Resolved)
	if err != nil {
		return nil, "", err
	}
	command := append(provision.Enter(manifest), "--", "sh", "-c", provision.RunScript(manifest, argv))
	return command, ruleset, nil
}

// ResolvConf is retained for callers. TAP guests receive resolver addresses from
// provisioned guest configuration, not QEMU's host /etc/resolv.conf.
func ResolvConf(schema.AppConfig) string { return "" }

func shellQuote(value string) string { return provision.Quote(value) }
func shellJoin(argv []string) string { return provision.ShellJoin(argv) }

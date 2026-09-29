// Package netenforce attaches containers to explicitly provisioned namespaces.
package netenforce

import (
	"fmt"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/nftrules"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

var _ ports.NetEnforcer = Enforcer{}

const DefaultNetfilterImage = "localhost/zinc/netfilter:local"

type Enforcer struct {
	Lookup        provision.Lookup
	Load          func(schema.AppConfig) (provision.Manifest, error)
	UserNamespace func() (uint64, error) // optional frozen Podman identity resolver
	ProcessID     func(string) (int, error)
}

func PodName(app string) string              { return app + "-pod" }
func filtered(cfg schema.AppConfig) bool     { return len(cfg.NetworkMeta.Interfaces) > 0 }
func NFTRuleset(cfg schema.AppConfig) string { return nftrules.Render(cfg) }

func (Enforcer) RunFlags(cfg schema.AppConfig) []string {
	if filtered(cfg) {
		return []string{"--pod", PodName(cfg.AppNameID)}
	}
	return []string{"--network", "none"}
}

func (enforcer Enforcer) manifest(cfg schema.AppConfig) (provision.Manifest, error) {
	if enforcer.Load != nil {
		return enforcer.Load(cfg)
	}
	return provision.Load(cfg)
}

func helperImage(opt options.HostOptions) string {
	if opt.NetfilterImage != "" {
		return opt.NetfilterImage
	}
	return DefaultNetfilterImage
}

func (enforcer Enforcer) Prepare(cfg schema.AppConfig, opt options.HostOptions) ([]ports.Command, error) {
	if !filtered(cfg) {
		return nil, nil
	}
	manifest, err := enforcer.manifest(cfg)
	if err != nil {
		return nil, err
	}
	plan, err := provision.Resolve(cfg, manifest, enforcer.Lookup)
	if err != nil {
		return nil, err
	}
	if err := provision.ReadyDNS(cfg, manifest); err != nil {
		return nil, err
	}
	ruleset, err := nftrules.RenderResolved(plan.Resolved)
	if err != nil {
		return nil, err
	}
	userNamespace, err := enforcer.userNamespaceMode(manifest)
	if err != nil {
		return nil, err
	}
	create := []string{"pod", "create", "--name", PodName(cfg.AppNameID),
		"--network", "ns:" + manifest.NetworkNamespace, "--userns", userNamespace}
	if len(manifest.DNSProxyAddresses) == 0 {
		create = append(create, "--dns", "none")
	}
	for _, address := range manifest.DNSProxyAddresses {
		create = append(create, "--dns", address)
	}
	return []ports.Command{
		{Args: helperArgs(manifest, userNamespace, helperImage(opt), provision.ApplyScript(manifest)), Stdin: ruleset, Desc: "verify provisioned namespace and lock nft before app"},
		{Args: create, Desc: "attach pod to locked provisioned namespace"},
	}, nil
}

func helperArgs(manifest provision.Manifest, userNamespace, image, script string) []string {
	return []string{"run", "--rm", "-i", "--pull", "never", "--network", "ns:" + manifest.NetworkNamespace,
		"--userns", userNamespace, "--user", "0", "--security-opt", "no-new-privileges",
		"--cap-drop", "all", "--cap-add", "NET_ADMIN", image, "sh", "-c", script}
}

func (enforcer Enforcer) Teardown(cfg schema.AppConfig) []ports.Command {
	if !filtered(cfg) {
		return []ports.Command{{Args: []string{"rm", "-f", "--ignore", cfg.AppNameID}, Desc: "remove isolated app"}}
	}
	steps := []ports.Command{{Args: []string{"pod", "rm", "-f", "--ignore", PodName(cfg.AppNameID)}, Desc: "remove app pod"}}
	manifest, err := enforcer.manifest(cfg)
	if err != nil {
		return append(steps, failedCommand(err))
	}
	userNamespace, err := enforcer.userNamespaceMode(manifest)
	if err != nil {
		return append(steps, failedCommand(err))
	}
	return append(steps, ports.Command{Args: helperArgs(manifest, userNamespace, DefaultNetfilterImage, provision.CloseScript(manifest)), Desc: "close provisioned namespace; preserve provisioner topology"})
}

func failedCommand(err error) ports.Command {
	return ports.Command{Args: []string{"run", "--rm", "--network", "none", "--pull", "never", "--cap-drop", "all",
		DefaultNetfilterImage, "sh", "-c", "printf '%s\\n' " + provision.Quote(err.Error()) + " >&2; exit 1"}, Desc: fmt.Sprintf("network operation refused: %v", err)}
}

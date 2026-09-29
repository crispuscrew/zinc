package netenforce

import (
	"fmt"

	provision "github.com/crispuscrew/zinc/common/adapters/network"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

func (enforcer Enforcer) Counters(cfg schema.AppConfig, opt options.HostOptions) (ports.Command, bool) {
	if !filtered(cfg) {
		return ports.Command{}, false
	}
	manifest, err := enforcer.manifest(cfg)
	if err != nil {
		return failedCommand(err), true
	}
	userNamespace, err := enforcer.userNamespaceMode(manifest)
	if err != nil {
		return failedCommand(err), true
	}
	resolve := enforcer.ProcessID
	if resolve == nil {
		resolve = containerProcess
	}
	process, err := resolve(cfg.AppNameID)
	if err != nil {
		return failedCommand(err), true
	}
	if process <= 0 {
		return failedCommand(fmt.Errorf("container %s has no running process", cfg.AppNameID)), true
	}
	script := "set -eu\n" + provision.Preflight(manifest) + "nft -j list table inet zinc\n"
	args := helperArgs(manifest, userNamespace, helperImage(opt), script)
	for index, argument := range args {
		if argument == "--network" {
			// Direct namespace joins avoid a forbidden dependency on an app in a pod.
			// Preflight binds the observed process namespace to the manifest inode.
			args[index+1] = fmt.Sprintf("ns:/proc/%d/ns/net", process)
			break
		}
	}
	return ports.Command{Args: args, Desc: "read installed provisioned policy counters"}, true
}

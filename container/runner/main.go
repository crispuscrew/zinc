// Command zcr runs canonical container app definitions via Podman.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/crispuscrew/zinc/common/adapters/dnsproxy"
	"github.com/crispuscrew/zinc/container/runner/adapters/host"
	"github.com/crispuscrew/zinc/container/runner/adapters/notifyfilter"
	"github.com/crispuscrew/zinc/container/runner/adapters/pipewirectx"
	"github.com/crispuscrew/zinc/container/runner/adapters/waylandctx"
	"github.com/crispuscrew/zinc/container/runner/wire"
)

const usage = `usage: zcr <command> [args]
  run <app[@instance]> [--instance NAME] [--exec] [-v HOST:CONTAINER[:OPTIONS]]...
                            print the launch plan, or launch it (--exec)
  build <app>               build the derived image (Install or CreatorFlags)
  validate <app>            validate and report warnings
  stop|restart|inspect <app>
  logs <app> [-f]
  term <app> [--shell]      open an attached terminal
  ps                        list running apps
  net [app[@instance]] [--json]
  where <app[@instance]> [--json]
  bus [--json]
  recheck <app>             check whether SourceTag moved
  image search <term> | resolve <ref>
  dns-proxy --config FILE --control-socket PATH --listen IP[:PORT]...
  version
<app> is a store name or a path (has '/' or ends in .yaml).`

var version = "dev"

func versionString() string {
	if version != "dev" && version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zcr: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("%s", usage)
	}
	if argv[0] == "version" || argv[0] == "--version" {
		fmt.Println("zcr " + versionString())
		return nil
	}
	if argv[0] == "dns-proxy" {
		return dnsproxy.Command(argv[1:])
	}
	svc, err := wire.DefaultService()
	if err != nil {
		return err
	}
	opt := host.Options()
	command, rest := argv[0], argv[1:]
	switch command {
	case "run":
		return cmdRun(svc, opt, rest)
	case "build":
		return cmdBuild(svc, rest)
	case "validate":
		return cmdValidate(svc, rest)
	case "stop", "restart", "inspect":
		return cmdLifecycle(svc, opt, command, rest)
	case "logs":
		return cmdLogs(svc, rest)
	case "term":
		return cmdTerm(svc, opt, rest)
	case "__term":
		return cmdTermWaiter(svc, opt, rest)
	case waylandctx.HoldCommand:
		return cmdWaylandHolder(opt, rest)
	case pipewirectx.HoldCommand:
		return cmdPipeWireHolder(opt, rest)
	case notifyfilter.HoldCommand:
		return cmdNotifyFilter(svc, opt, rest)
	case "__supervise":
		return cmdSupervise(svc, rest)
	case "ps":
		return cmdPs(svc)
	case "net":
		return cmdNet(svc, opt, rest)
	case "where":
		return cmdWhere(svc, opt, rest)
	case "bus":
		return cmdBus(svc, opt, rest)
	case "recheck":
		return cmdRecheck(svc, rest)
	case "image":
		return cmdImage(svc, rest)
	default:
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
}

// Command zvr runs VM apps using shared app intent and separate runtime options.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/crispuscrew/zinc/common/adapters/dnsproxy"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/audioholder"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/disk"
	"github.com/crispuscrew/zinc/virtualization/runner/app"
)

var version = "dev"

const usage = `usage:
  zvr run <app|path.yaml> [--dry-run] [runtime options]
  zvr validate <app|path.yaml> [runtime options]
  zvr stop <app> [--force]
  zvr ps
  zvr status <app>
  zvr reset <app> --confirm
  zvr pin <image.qcow2>
  zvr install --disk PATH --media ISO [options]
  zvr net <app> [--json]
  zvr console <app>
  zvr dns-proxy --config FILE --control-socket PATH --listen IP[:PORT]...
  zvr version

Runtime options (never written to app YAML):
  --runtime-options FILE   select an explicit options JSON file
  --base-digest SHA256     override the authorized base pin for this invocation
  --disk GiB              initial overlay size; 0 preserves the base size
  --display MODE          None, Window, Accelerated, Compatible; empty infers app intent
  --devices PROFILE       Virtio or Compatible
  --media ISO             attach a read-only disc; repeatable, replaces stored list
  --forward HOST:GUEST[/TCP|UDP][@NIC]  repeatable, loopback by default
  --clear-media           clear stored discs for this invocation
  --clear-forwards        clear stored forwards for this invocation
  --runner-arg ARG        raw QEMU argument; repeatable, warns before execution`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zvr: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("%s", usage)
	}
	command, rest := argv[0], argv[1:]
	switch command {
	case "help", "--help", "-h":
		fmt.Println(usage)
		return nil
	case "version", "--version":
		fmt.Println("zvr " + versionString())
		return nil
	case "pin":
		return cmdPin(rest)
	case "install":
		return cmdInstall(rest)
	case "dns-proxy":
		return dnsproxy.Command(rest)
	case audioholder.Command:
		if len(rest) != 2 {
			return fmt.Errorf("invalid internal audio-holder arguments")
		}
		return audioholder.Hold(rest[0], rest[1])
	}
	svc, err := service()
	if err != nil {
		return err
	}
	switch command {
	case "run":
		return cmdRun(svc, rest)
	case app.SupervisorCommand:
		if len(rest) != 2 {
			return fmt.Errorf("invalid internal supervisor arguments")
		}
		return svc.HoldSupervisor(rest[0], rest[1])
	case "validate":
		return cmdValidate(svc, rest)
	case "stop":
		return cmdStop(svc, rest)
	case "ps":
		return cmdPS(svc, rest)
	case "status":
		return cmdStatus(svc, rest)
	case "reset":
		return cmdReset(svc, rest)
	case "console":
		return cmdConsole(svc, rest)
	case "__console-session":
		if len(rest) < 1 || len(rest) > 2 || len(rest) == 2 && rest[1] != "--background" {
			return fmt.Errorf("invalid internal console-session arguments")
		}
		return svc.ConsoleSession(rest[0], len(rest) == 2)
	case "net":
		return cmdNet(svc, rest)
	default:
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
}

func cmdPin(argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zvr pin <image.qcow2>")
	}
	digest, err := disk.Digest(argv[0])
	if err == nil {
		fmt.Println(digest)
	}
	return err
}

func versionString() string {
	if version != "dev" && version != "" {
		return version
	}
	if info, valid := debug.ReadBuildInfo(); valid && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

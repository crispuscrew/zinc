// Command zc authors Zinc apps and delegates execution to their runtime.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/runner"
	"github.com/crispuscrew/zinc/creator/internal/store"
	"github.com/crispuscrew/zinc/creator/internal/tui"
)

const usage = `usage: zc <command> [args]
  tui                               keyboard-first app manager
  new <name> --image <img> [options]  author an app; --help lists all fields
  new <name> --vm --image <disk> --base-digest sha256:... [options]
  init [--force]                    seed example apps
  list
  validate <name|app.yaml> [--resolved]
  delete <name>
  keys list|show|set <s>|edit|validate|path
  compose export <name> [-o file] | import <file> [--dry-run]
  run <name|app.yaml> [--exec]       print a plan, or launch
  build <name|app.yaml>             build a container's derived image
  stop|restart|inspect <name>
  logs <name> [-f]
  term <name> [--shell]             open an attached terminal / VM console
  image search <term>|resolve <ref>
  version

VM definitions also own runtime options under zinc/runtime/vm/<name>.json.
Raw CreatorFlags/RunnerFlags are ordered backend argv, not shell commands.`

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
		fmt.Fprintln(os.Stderr, "zc: "+err.Error())
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("%s", usage)
	}
	command, rest := argv[0], argv[1:]
	switch command {
	case "version", "--version":
		fmt.Println("zc " + versionString())
		return nil
	case "--help", "-h":
		fmt.Println(usage)
		return nil
	case "keys":
		return cmdKeys(rest)
	case "image":
		return runner.Passthrough(argv...)
	}
	sto, err := store.Default()
	if err != nil {
		return err
	}
	svc := backend.New(sto)
	switch command {
	case "run", "build", "stop", "restart", "inspect", "logs", "term":
		return delegate(svc, command, rest)
	case "tui":
		_, err := tea.NewProgram(tui.New(svc, loadKeys()), tea.WithAltScreen()).Run()
		return err
	case "new":
		return cmdNew(svc, rest)
	case "init":
		return cmdInit(svc, rest)
	case "list":
		return cmdList(svc)
	case "validate":
		return cmdValidate(svc, rest)
	case "delete":
		return cmdDelete(svc, rest)
	case "compose":
		return cmdCompose(svc, rest)
	default:
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
}

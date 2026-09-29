package main

import (
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/adapters/podman"
	"github.com/crispuscrew/zinc/container/runner/adapters/waylandctx"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/domain/session"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

func cmdRun(svc app.Service, opt options.HostOptions, argv []string) error {
	name, execute, volumes, err := parseRunArgs(argv)
	if err != nil {
		return err
	}
	cfg, err := loadLaunchable(svc, name)
	if err != nil {
		return err
	}
	cfg.Volumes = append(cfg.Volumes, volumes...)
	if execute {
		return svc.Launch(cfg, opt)
	}
	if err := validate.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config %s:\n%w", name, err)
	}
	for _, warning := range validate.Warnings(cfg) {
		fmt.Println("# WARNING: " + warning)
	}
	if derived.HasInstall(cfg) {
		printPlan([]ports.Command{{Args: podman.ImageBuildArgs(cfg), Stdin: derived.DerivedContainerfile(cfg), Desc: "build derived image first (auto on run when stale)"}})
	}
	if waylandctx.Applies(cfg, opt) {
		fmt.Println("# a real launch first registers a per-instance Wayland socket with the compositor")
	}
	for _, device := range []schema.AudioDevice{cfg.AudioMeta.Playback, cfg.AudioMeta.Microphone, cfg.AudioMeta.Monitor} {
		if device.PipeWireDefault || len(device.PipeWireDevices) > 0 {
			fmt.Println("# PipeWire mount below is a placeholder; a real launch must establish a restricted per-app socket")
			break
		}
	}
	plan, err := svc.Plan(cfg, opt)
	if err != nil {
		return err
	}
	printPlan(plan)
	if cfg.StartConditions.Attached {
		printPlan([]ports.Command{{Args: podman.ExecArgs(cfg.AppNameID, termCmd(cfg), session.Environment(cfg.StartConditions)), Desc: "each terminal attaches to the holder"}})
	}
	return nil
}

func termCmd(cfg schema.AppConfig) []string { return session.Command(cfg.StartConditions) }

func printPlan(plan []ports.Command) {
	for _, command := range plan {
		fmt.Println("# " + strings.ReplaceAll(command.Desc, "\n", "\n# "))
		line := "podman " + strings.Join(quoteForDisplay(command.Args), " ")
		if command.Stdin == "" {
			fmt.Println(line)
			continue
		}
		// A quoted here-document prevents stdin (including install scripts) expanding on the host.
		delimiter := "ZINC_STDIN"
		for strings.Contains(command.Stdin, delimiter) {
			delimiter += "_"
		}
		fmt.Println(line + " <<'" + delimiter + "'")
		fmt.Print(command.Stdin)
		if !strings.HasSuffix(command.Stdin, "\n") {
			fmt.Println()
		}
		fmt.Println(delimiter)
	}
}

func quoteForDisplay(args []string) []string {
	out := make([]string, len(args))
	for index, arg := range args {
		safe := arg != "" && strings.IndexFunc(arg, func(char rune) bool {
			return !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("_@%+=:,./-", char))
		}) < 0
		if safe {
			out[index] = arg
		} else {
			out[index] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
	}
	return out
}

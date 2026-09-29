package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/creator/internal/backend"
	"github.com/crispuscrew/zinc/creator/internal/compose"
)

const composeUsage = `usage:
  zc compose export <name|app.yaml> [-o compose.yaml]
  zc compose import <compose.yaml> [--service name] [--dry-run]
Both directions report losses. Compose does not reproduce Zinc's network policy.
Import never infers outbound access or raw backend privilege flags.`

func splitLeadingArg(argv []string) (string, []string, error) {
	if len(argv) == 0 || strings.HasPrefix(argv[0], "-") {
		return "", nil, fmt.Errorf("%s", composeUsage)
	}
	return argv[0], argv[1:], nil
}

func cmdCompose(service backend.Service, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("%s", composeUsage)
	}
	switch argv[0] {
	case "export":
		return cmdComposeExport(service, argv[1:])
	case "import":
		return cmdComposeImport(service, argv[1:])
	default:
		return fmt.Errorf("unknown compose subcommand %q\n%s", argv[0], composeUsage)
	}
}

func cmdComposeExport(service backend.Service, argv []string) error {
	name, rest, err := splitLeadingArg(argv)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("compose export", flag.ContinueOnError)
	output := flags.String("o", "", "output path (default stdout)")
	if err := flags.Parse(rest); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	cfg, err := loadApp(service, name)
	if err != nil {
		return err
	}
	if err := validate.Validate(cfg); err != nil {
		return err
	}
	project, notes, err := compose.FromApp(cfg)
	if err != nil {
		return err
	}
	data, err := marshalProject(project)
	if err != nil {
		return err
	}
	if *output == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(*output, data, 0o600); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", *output)
	}
	printNotes(os.Stderr, "not represented", notes)
	return nil
}

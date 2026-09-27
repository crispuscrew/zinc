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

func cmdComposeImport(service backend.Service, argv []string) error {
	path, rest, err := splitLeadingArg(argv)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("compose import", flag.ContinueOnError)
	only := flags.String("service", "", "select one service")
	dryRun := flags.Bool("dry-run", false, "print definitions without saving")
	if err := flags.Parse(rest); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	project, err := readProject(path)
	if err != nil {
		return err
	}
	apps := compose.ToApps(project)
	if *only != "" {
		apps = selectService(apps, *only)
	}
	if len(apps) == 0 {
		return fmt.Errorf("no matching services in %s", path)
	}
	var failed []string
	for _, app := range apps {
		if err := importOne(service, app, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "zc: %s: %v\n", app.Config.AppNameID, err)
			failed = append(failed, app.Config.AppNameID)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d services could not be imported: %s", len(failed), len(apps), strings.Join(failed, ", "))
	}
	return nil
}

func importOne(service backend.Service, app compose.App, dryRun bool) error {
	cfg := app.Config
	printNotes(os.Stderr, "note", app.Notes)
	pinned, note, err := pinImage(cfg.ImageMeta.Image)
	if err != nil {
		return err
	}
	cfg.ImageMeta.Image = pinned
	if note != "" {
		fmt.Fprintln(os.Stderr, "note: "+note)
	}
	if err := validate.Validate(cfg); err != nil {
		return err
	}
	if dryRun {
		data, err := service.Marshal(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("# --- %s ---\n%s\n", cfg.AppNameID, data)
		return nil
	}
	if err := service.Create(cfg, nil); err != nil {
		return err
	}
	fmt.Printf("imported %s -> %s\n", cfg.AppNameID, service.Path(cfg.AppNameID))
	return nil
}

func selectService(apps []compose.App, wanted string) []compose.App {
	for _, app := range apps {
		if app.Service == wanted || app.Config.AppNameID == wanted {
			return []compose.App{app}
		}
	}
	return nil
}

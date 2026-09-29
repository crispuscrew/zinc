package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/crispuscrew/zinc/container/runner/app"
)

func cmdImage(svc app.Service, argv []string) error {
	if len(argv) != 2 {
		return fmt.Errorf("usage: zcr image search <term> | zcr image resolve <ref>")
	}
	switch argv[0] {
	case "search":
		results, err := svc.Search(argv[1])
		if err != nil {
			return err
		}
		if len(results) == 0 {
			fmt.Println("no images found")
		}
		for _, result := range results {
			fmt.Printf("%s\t%s\n", result.Name, result.Description)
		}
		return nil
	case "resolve", "pin":
		pinned, err := svc.Resolve(argv[1])
		if err != nil {
			return err
		}
		fmt.Println(pinned)
		return nil
	default:
		return fmt.Errorf("unknown image subcommand %q (want search|resolve)", argv[0])
	}
}

func cmdRecheck(svc app.Service, argv []string) error {
	if len(argv) != 1 {
		return fmt.Errorf("usage: zcr recheck <app>")
	}
	cfg, err := loadApp(svc, argv[0])
	if err != nil {
		return err
	}
	tag := strings.TrimSpace(cfg.ImageMeta.SourceTag)
	if tag == "" {
		return fmt.Errorf("%s: no ImageMeta.SourceTag recorded, so there is no tag to re-resolve", cfg.AppNameID)
	}
	if strings.Contains(tag, "@sha256:") {
		return fmt.Errorf("%s: ImageMeta.SourceTag is a digest (%s), not a tag", cfg.AppNameID, tag)
	}
	resolved, err := svc.Resolve(tag)
	if err != nil {
		return fmt.Errorf("%s: re-resolving %s: %w", cfg.AppNameID, tag, err)
	}
	fmt.Printf("tag: %s\npinned: %s\ncurrent: %s\n", tag, cfg.ImageMeta.Image, resolved)
	if resolved == cfg.ImageMeta.Image {
		fmt.Println("status: current")
		return nil
	}
	fmt.Println("status: moved")
	os.Exit(2)
	return nil
}

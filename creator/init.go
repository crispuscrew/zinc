package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/crispuscrew/zinc/creator/internal/backend"
)

func cmdInit(svc backend.Service, argv []string) error {
	force := false
	for _, arg := range argv {
		if arg != "--force" {
			return fmt.Errorf("unknown flag %q (usage: zc init [--force])", arg)
		}
		force = true
	}
	var written, skipped []string
	for _, seed := range seedApps {
		if svc.Exists(seed.name) && !force {
			skipped = append(skipped, seed.name)
			continue
		}
		draft, err := os.CreateTemp("", "zinc-seed-*.yaml")
		if err != nil {
			return err
		}
		_, writeErr := draft.WriteString(seed.yaml)
		closeErr := draft.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			os.Remove(draft.Name())
			return err
		}
		cfg, loadErr := svc.LoadFile(draft.Name())
		removeErr := os.Remove(draft.Name())
		if err := errors.Join(loadErr, removeErr); err != nil {
			return fmt.Errorf("seed %s: %w", seed.name, err)
		}
		if force {
			err = svc.Save(cfg)
		} else {
			err = svc.Create(cfg, nil)
		}
		if err != nil {
			return fmt.Errorf("seed %s: %w", seed.name, err)
		}
		written = append(written, seed.name)
	}
	for _, name := range written {
		fmt.Printf("created %s -> %s\n", name, svc.Path(name))
	}
	if len(skipped) > 0 {
		fmt.Printf("kept existing: %s (use --force to replace)\n", strings.Join(skipped, ", "))
	}
	if len(written) > 0 {
		fmt.Println("next:\n  podman pull docker.io/library/alpine@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1\n  set ZINC_TERMINAL if needed\n  zc validate example-shell\n  zc run example-shell --exec")
	}
	return nil
}

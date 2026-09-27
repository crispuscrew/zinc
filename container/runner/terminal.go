package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
)

func cmdTerm(svc app.Service, opt options.HostOptions, argv []string) error {
	name, shell, err := parseTermArgs(argv)
	if err != nil {
		return err
	}
	cfg, err := loadLaunchable(svc, name)
	if err != nil {
		return err
	}
	return svc.OpenTerminal(cfg, opt, shell)
}

func cmdTermWaiter(svc app.Service, opt options.HostOptions, argv []string) error {
	name, shell, err := parseTermArgs(argv)
	if err != nil {
		return err
	}
	var request app.TerminalRequest
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("terminal request: %w", err)
	}
	if request.Config.AppNameID != name {
		return fmt.Errorf("terminal request: app identity mismatch")
	}
	return svc.Term(request.Config, request.Options, shell)
}

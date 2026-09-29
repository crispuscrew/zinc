package main

import (
	"os"
	"strings"

	"github.com/crispuscrew/zinc/common/adapters/dnsproxy"
	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/virtualization/runner/adapters/fs"
	"github.com/crispuscrew/zinc/virtualization/runner/app"
	"github.com/crispuscrew/zinc/virtualization/runner/domain/paths"
)

func service() (app.Service, error) {
	store, err := fs.Default()
	if err != nil {
		return app.Service{}, err
	}
	layout, err := paths.Default()
	if err != nil {
		return app.Service{}, err
	}
	svc := app.New(store, layout)
	svc.AudioRuntimeDir = os.Getenv("XDG_RUNTIME_DIR")
	svc.Lookup = dnsproxy.Lookup
	return svc, nil
}

func loadApp(svc app.Service, name string) (schema.AppConfig, error) {
	if strings.Contains(name, "/") || strings.HasSuffix(name, ".yaml") {
		return svc.Store.LoadFileResolved(name)
	}
	return svc.Store.LoadResolved(name)
}

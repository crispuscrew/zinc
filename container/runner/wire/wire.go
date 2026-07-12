// Package wire is the runner's composition root: it assembles the concrete adapters
// (podman runtime/builder/resolver, the netenforce egress enforcer, the fs store)
// into a ready-to-use app.Service. It is deliberately the ONE place that imports
// every adapter, kept out of the domain/ports/app layers so those stay
// adapter-agnostic — the hexagon's center never names a concrete edge.
//
// Front-ends call Service / DefaultService to get a fully wired facade; swapping an
// adapter (e.g. a future non-pasta egress enforcer) is a one-line change here,
// nowhere else.
package wire

import (
	"github.com/crispuscrew/zinc/container/runner/adapters/fs"
	"github.com/crispuscrew/zinc/container/runner/adapters/netenforce"
	"github.com/crispuscrew/zinc/container/runner/adapters/podman"
	"github.com/crispuscrew/zinc/container/runner/app"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

// Service wires a given store with the podman adapters and the egress enforcer.
// Tests pass an in-temp-dir store; production uses DefaultService.
func Service(store ports.Store) app.Service {
	return app.New(store, podman.Runtime{}, podman.Builder{}, podman.Resolver{}, netenforce.Enforcer{})
}

// DefaultService builds the production service against the standard on-disk store
// (~/.config/zinc/apps).
func DefaultService() (app.Service, error) {
	store, err := fs.Default()
	if err != nil {
		return app.Service{}, err
	}
	return Service(store), nil
}

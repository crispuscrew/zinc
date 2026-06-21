// Package wire is HyprZinc's composition root: it assembles the concrete adapters
// (podman runtime/builder/resolver, the netenforce strategies, the fs store) into a
// ready-to-use app.Service. It is deliberately the ONE place that imports every
// adapter, kept out of the domain/ports/app layers so those stay adapter-agnostic —
// the hexagon's center never names a concrete edge.
//
// Front-ends (hzc, hzl) call Service / DefaultService to get a fully wired facade;
// swapping an adapter (e.g. a future non-pasta egress enforcer) is a one-line change
// here, nowhere else.
package wire

import (
	"github.com/crispuscrew/hyprzinc/core/adapters/fs"
	"github.com/crispuscrew/hyprzinc/core/adapters/netenforce"
	"github.com/crispuscrew/hyprzinc/core/adapters/podman"
	"github.com/crispuscrew/hyprzinc/core/app"
	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/core/ports"
)

// Enforcers is the network-egress strategy set, keyed by network mode. This map is
// where the swappable traffic-control mechanisms are registered: a new mechanism is
// a new NetEnforcer adapter added here (or replacing pasta), and nothing else moves.
func Enforcers() map[string]ports.NetEnforcer {
	return map[string]ports.NetEnforcer{
		domain.NetworkNone:      netenforce.None{},
		domain.NetworkPasta:     netenforce.Pasta{},
		domain.NetworkContainer: netenforce.Container{},
	}
}

// Service wires a given store with the podman adapters and the egress enforcers.
// Tests pass an in-temp-dir store; production uses DefaultService.
func Service(store ports.Store) app.Service {
	return app.New(store, podman.Runtime{}, podman.Builder{}, podman.Resolver{}, Enforcers())
}

// DefaultService builds the production service against the standard on-disk store
// (~/.config/hyprzinc/apps).
func DefaultService() (app.Service, error) {
	store, err := fs.Default()
	if err != nil {
		return app.Service{}, err
	}
	return Service(store), nil
}

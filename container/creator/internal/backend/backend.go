// Package backend is the creator's single dependency surface for the CLI and TUI: it
// authors app files through the embedded store and runs them by delegating to the zcr
// binary (the runner package). Keeping both behind one type means the front-ends call
// svc.Load / svc.Save (authoring, local) and svc.Launch / svc.Stop (runtime, via zcr)
// uniformly, without knowing which side of the zcc/zcr split each action lives on.
package backend

import (
	"github.com/crispuscrew/zinc/container/creator/internal/runner"
	"github.com/crispuscrew/zinc/container/creator/internal/store"
)

// Result re-exports an image-search hit so the front-ends need not import the runner
// package directly.
type Result = runner.Result

// Service is the creator's backend. The embedded *store.Store promotes the authoring
// methods (List, Load, LoadFile, Save, Delete, Exists, Path, Marshal); the runtime
// methods below forward to zcr.
type Service struct {
	*store.Store
}

// New assembles the backend around a store (the composition root builds the store, then
// wraps it here).
func New(sto *store.Store) Service {
	return Service{Store: sto}
}

// Launch starts the app detached, via zcr.
func (Service) Launch(name string) error { return runner.Launch(name) }

// Stop tears the app's pod down, via zcr.
func (Service) Stop(name string) error { return runner.Stop(name) }

// Plan returns the app's launch plan (dry run) as text, via zcr.
func (Service) Plan(name string) (string, error) { return runner.Plan(name) }

// Build (re)builds the app's derived image, via zcr, returning its output.
func (Service) Build(name string) (string, error) { return runner.Build(name) }

// OpenTerminal opens one more terminal for a multiterminal app, via zcr.
func (Service) OpenTerminal(name string, shell bool) error { return runner.OpenTerminal(name, shell) }

// Logs returns a snapshot of the app's logs, via zcr.
func (Service) Logs(name string) (string, error) { return runner.Logs(name) }

// Resolve pins an image reference to its digest form, via zcr.
func (Service) Resolve(ref string) (string, error) { return runner.Resolve(ref) }

// Search finds images by term, via zcr.
func (Service) Search(term string) ([]Result, error) { return runner.Search(term) }

// Running returns the set of apps podman reports as up, via zcr.
func (Service) Running() (map[string]bool, error) { return runner.Running() }

// Rename moves an app definition on disk: load the old, re-key its AppNameID, save the
// new (which re-validates), then drop the old file. A no-op rename (same name) still
// re-saves and keeps the file.
func (svc Service) Rename(from, to string) error {
	cfg, err := svc.Load(from)
	if err != nil {
		return err
	}
	cfg.AppNameID = to
	if err := svc.Save(cfg); err != nil {
		return err
	}
	if from != to {
		return svc.Delete(from)
	}
	return nil
}

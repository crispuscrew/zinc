// Package app is the runner's application layer - the hexagon's "inside". A Service
// orchestrates a launch by composing the ports (Store, Runtime, ImageBuilder,
// ImageResolver, NetEnforcer) and depends on none of their concrete adapters. The
// front-ends build the adapters, hand them to New, and drive everything through this
// one facade.
//
// This is where the launch sequence lives - validate, build the derived image if
// needed, run the egress lock-down through the NetEnforcer, then start the app - so
// there is exactly one launch path to get right (docs/architecture.md section 9.1, section 13).
package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/crispuscrew/zinc/common/domain/schema/validate"
	"github.com/crispuscrew/zinc/container/runner/domain/derived"
	"github.com/crispuscrew/zinc/container/runner/domain/options"
	"github.com/crispuscrew/zinc/container/runner/ports"
)

// Service is the application facade. Construct it with New.
type Service struct {
	store    ports.Store
	runtime  ports.Runtime
	builder  ports.ImageBuilder
	resolver ports.ImageResolver
	net      ports.NetEnforcer
}

// New wires the ports into a Service.
func New(store ports.Store, runtime ports.Runtime, builder ports.ImageBuilder, resolver ports.ImageResolver, net ports.NetEnforcer) Service {
	return Service{store: store, runtime: runtime, builder: builder, resolver: resolver, net: net}
}

// Plan returns the ordered runtime commands a launch would run, without running them
// - the NetEnforcer's pre-steps (establish + lock the netns) followed by the app
// container. Used for dry-run so what will happen is fully visible.
func (svc Service) Plan(cfg schema.AppConfig, opt options.HostOptions) ([]ports.Command, error) {
	if err := validate.Validate(cfg); err != nil { // never compose commands from unvalidated config (section 3)
		return nil, fmt.Errorf("%s: %w", cfg.AppNameID, err)
	}
	if err := checkNetwork(cfg); err != nil {
		return nil, err
	}
	appArgs, err := svc.runtime.AppRunArgs(cfg, opt, svc.net.RunFlags(cfg))
	if err != nil {
		return nil, err
	}
	desc := "run " + cfg.AppNameID
	if cfg.StartConditions.Multiterminal {
		desc = "run holder for " + cfg.AppNameID + " (terminals exec in)"
	}
	return append(svc.net.Prepare(cfg, opt), ports.Command{Args: appArgs, Desc: desc}), nil
}

// Launch validates cfg, auto-starts its depends_on apps (section 6.6), ensures its derived
// image (if ImageMeta.Install is set), runs the egress lock-down through the
// NetEnforcer (fail-closed: a half-built netns is torn down on any error), then
// starts the app container detached. A multiterminal app launches by opening its
// first terminal instead (the holder + a `podman exec`).
func (svc Service) Launch(cfg schema.AppConfig, opt options.HostOptions) error {
	return svc.launch(cfg, opt, nil)
}

// launch is Launch's recursive core. chain is the stack of apps already mid-launch
// (root → cfg's parent); it lets depends_on auto-start detect cycles. The public
// Launch starts the recursion with a nil chain.
func (svc Service) launch(cfg schema.AppConfig, opt options.HostOptions, chain []string) error {
	if err := validate.Validate(cfg); err != nil { // launch-time check catches drift (section 3)
		return fmt.Errorf("%s: %w", cfg.AppNameID, err)
	}
	if err := checkNetwork(cfg); err != nil { // fail closed on not-yet-supported network shapes
		return err
	}
	if err := svc.startDependencies(cfg, opt, chain); err != nil { // section 6.6: dependencies first
		return err
	}
	if cfg.StartConditions.Multiterminal {
		return svc.OpenTerminal(cfg, opt, false) // ensures the image itself
	}
	if err := svc.ensureImage(cfg); err != nil {
		return err
	}
	steps := svc.net.Prepare(cfg, opt)
	for _, cmd := range steps {
		if err := svc.runtime.Exec(cmd); err != nil {
			return errors.Join(fmt.Errorf("launch %s (%s): %w", cfg.AppNameID, cmd.Desc, err), svc.teardown(cfg, len(steps) > 0))
		}
	}
	appArgs, err := svc.runtime.AppRunArgs(cfg, opt, svc.net.RunFlags(cfg))
	if err != nil {
		return errors.Join(err, svc.teardown(cfg, len(steps) > 0))
	}
	// StartApp returns before `podman run` succeeds; if the app dies post-fork the
	// prepared pod/netns would leak, so onFail tears it down from the reaping goroutine.
	onFail := func() { _ = svc.teardown(cfg, len(steps) > 0) }
	if err := svc.runtime.StartApp(cfg, opt, appArgs, onFail); err != nil {
		return errors.Join(err, svc.teardown(cfg, len(steps) > 0))
	}
	return nil
}

// Stop tears a running app down via the enforcer's Teardown (the pod and its filtered
// netns for a filtered app, the container otherwise). Output is captured, so it is
// safe to call from a UI.
func (svc Service) Stop(cfg schema.AppConfig) error {
	return svc.runtime.Exec(ports.Command{Args: svc.net.Teardown(cfg), Desc: "stop " + cfg.AppNameID})
}

// teardown removes a half-built netns after a failed launch (fail-closed). It only
// fires when the enforcer had pre-steps to undo (a filtered app); an unfiltered app
// has nothing half-built. It returns any error so a failed/leaked teardown is visible
// to the caller (joined into the launch error) rather than silently swallowed.
func (svc Service) teardown(cfg schema.AppConfig, hadSteps bool) error {
	if !hadSteps {
		return nil
	}
	return svc.runtime.Exec(ports.Command{Args: svc.net.Teardown(cfg), Desc: "teardown " + cfg.AppNameID})
}

// Rename changes an app's identity from oldName to newName. There is no atomic file
// rename, because the name lives in two places - the filename and AppNameID inside
// the YAML - so this loads the definition, rewrites AppNameID, saves it under the new
// name (which re-validates the name), and removes the old definition.
//
// It refuses to overwrite an existing app, and to rename a running one - its
// container is named after the old name and would be orphaned (the renamed definition
// could no longer stop it); stop it first.
func (svc Service) Rename(oldName, newName string) error {
	oldName, newName = strings.TrimSpace(oldName), strings.TrimSpace(newName)
	switch {
	case newName == "":
		return fmt.Errorf("rename %s: new name must not be empty", oldName)
	case newName == oldName:
		return fmt.Errorf("rename %s: new name is unchanged", oldName)
	case svc.store.Exists(newName):
		return fmt.Errorf("rename %s: %q already exists", oldName, newName)
	}
	if running, err := svc.runtime.Running(); err == nil && running[oldName] {
		return fmt.Errorf("rename %s: app is running - stop it first (its container is named %q)", oldName, oldName)
	}
	cfg, err := svc.store.Load(oldName)
	if err != nil {
		return fmt.Errorf("rename %s: %w", oldName, err)
	}
	cfg.AppNameID = newName
	if err := svc.store.Save(cfg); err != nil { // validates the new name before anything is removed
		return fmt.Errorf("rename %s -> %s: %w", oldName, newName, err)
	}
	if err := svc.store.Delete(oldName); err != nil {
		return fmt.Errorf("rename %s -> %s: saved new definition but could not remove the old one: %w", oldName, newName, err)
	}
	return nil
}

// Build force-rebuilds an app's derived image (the explicit-rebuild path).
func (svc Service) Build(cfg schema.AppConfig) error { return svc.builder.Build(cfg) }

// ensureImage builds the derived image when ImageMeta.Install is set and the live
// image is missing or stale (its fingerprint label differs) - the auto-on-run trigger
// (section 9.1).
func (svc Service) ensureImage(cfg schema.AppConfig) error {
	if !derived.HasInstall(cfg) {
		return nil
	}
	want := derived.BuildFingerprint(cfg)
	if got, err := svc.builder.Fingerprint(derived.DerivedImageRef(cfg.AppNameID)); err == nil && got == want {
		return nil // already current
	}
	return svc.builder.Build(cfg)
}

// --- thin passthroughs so the front-ends drive everything through one facade ---

func (svc Service) List() ([]string, error)                    { return svc.store.List() }
func (svc Service) Load(name string) (schema.AppConfig, error) { return svc.store.Load(name) }
func (svc Service) Save(cfg schema.AppConfig) error            { return svc.store.Save(cfg) }
func (svc Service) Delete(name string) error                   { return svc.store.Delete(name) }
func (svc Service) Exists(name string) bool                    { return svc.store.Exists(name) }
func (svc Service) Path(name string) string                    { return svc.store.Path(name) }

func (svc Service) Marshal(cfg schema.AppConfig) ([]byte, error)   { return svc.store.Marshal(cfg) }
func (svc Service) LoadFile(path string) (schema.AppConfig, error) { return svc.store.LoadFile(path) }

func (svc Service) Search(term string) ([]ports.Result, error) { return svc.resolver.Search(term) }
func (svc Service) Resolve(ref string) (string, error)         { return svc.resolver.Resolve(ref) }

func (svc Service) Running() (map[string]bool, error)          { return svc.runtime.Running() }
func (svc Service) Logs(name string, tail int) (string, error) { return svc.runtime.Logs(name, tail) }

// Do runs a user-facing runtime command (restart/inspect/logs passthrough) with the
// host's stdio - for the CLI, where streaming output is wanted.
func (svc Service) Do(args []string) error { return svc.runtime.Do(args) }

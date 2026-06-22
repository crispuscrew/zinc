// Package app is HyprZinc's application layer — the hexagon's "inside". A Service
// orchestrates a launch by composing the ports (Store, Runtime, ImageBuilder,
// ImageResolver, and a per-mode NetEnforcer) and depends on none of their concrete
// adapters. The front-ends (hzc, hzl) build the adapters, hand them to New, and then
// drive everything through this one facade.
//
// This is where the launch sequence lives — validate, build the derived image if
// needed, run the egress lock-down through the NetEnforcer, then start the app — so
// there is exactly one launch path to get right (docs/architecture.md §9.1, §13).
package app

import (
	"errors"
	"fmt"

	"github.com/crispuscrew/hyprzinc/core/domain"
	"github.com/crispuscrew/hyprzinc/core/ports"
)

// Service is the application facade. Construct it with New.
type Service struct {
	store    ports.Store
	runtime  ports.Runtime
	builder  ports.ImageBuilder
	resolver ports.ImageResolver
	net      map[string]ports.NetEnforcer // keyed by domain.NetworkNone|Pasta|Container
}

// New wires the ports into a Service. net must hold an enforcer for every network
// mode an app may use (none/pasta/container); a missing one fails the launch with a
// clear error rather than a nil panic.
func New(store ports.Store, runtime ports.Runtime, builder ports.ImageBuilder, resolver ports.ImageResolver, net map[string]ports.NetEnforcer) Service {
	return Service{store: store, runtime: runtime, builder: builder, resolver: resolver, net: net}
}

func (svc Service) enforcer(cfg domain.AppConfig) (ports.NetEnforcer, error) {
	if enf := svc.net[cfg.Network.Mode]; enf != nil {
		return enf, nil
	}
	return nil, fmt.Errorf("%s: no network enforcer for mode %q", cfg.App.Name, cfg.Network.Mode)
}

// Plan returns the ordered runtime commands a launch would run, without running
// them — the NetEnforcer's pre-steps (establish + lock the netns) followed by the
// app container. Used by `hzc run` (dry-run) so what will happen is fully visible.
func (svc Service) Plan(cfg domain.AppConfig, opt domain.HostOptions) ([]ports.Command, error) {
	if err := domain.Validate(cfg); err != nil { // never compose commands from unvalidated config (§3)
		return nil, fmt.Errorf("%s: %w", cfg.App.Name, err)
	}
	enf, err := svc.enforcer(cfg)
	if err != nil {
		return nil, err
	}
	appArgs, err := svc.runtime.AppRunArgs(cfg, opt, enf.RunFlags(cfg))
	if err != nil {
		return nil, err
	}
	desc := "run " + cfg.App.Name
	if cfg.App.Multiterminal {
		desc = "run holder for " + cfg.App.Name + " (terminals exec in)"
	}
	return append(enf.Prepare(cfg, opt), ports.Command{Args: appArgs, Desc: desc}), nil
}

// Launch validates cfg, auto-starts its depends_on containers (§6.6), verifies a
// container-mode network target is present (§6.4), ensures its derived image (if
// app.install is set), runs the egress lock-down through the NetEnforcer
// (fail-closed: a half-built netns is torn down on any error), then starts the app
// container detached. A multiterminal app launches by opening its first terminal
// instead (the holder + a `podman exec`).
func (svc Service) Launch(cfg domain.AppConfig, opt domain.HostOptions) error {
	return svc.launch(cfg, opt, nil)
}

// launch is Launch's recursive core. chain is the stack of apps already mid-launch
// (root → cfg's parent); it lets depends_on auto-start detect cycles. The public
// Launch starts the recursion with a nil chain.
func (svc Service) launch(cfg domain.AppConfig, opt domain.HostOptions, chain []string) error {
	if err := domain.Validate(cfg); err != nil { // launch-time check catches drift (§3)
		return fmt.Errorf("%s: %w", cfg.App.Name, err)
	}
	if err := svc.startDependencies(cfg, opt, chain); err != nil { // §6.6: dependencies first
		return err
	}
	if err := svc.verifyNetTarget(cfg); err != nil { // §6.4: never attach to a missing netns
		return err
	}
	if cfg.App.Multiterminal {
		return svc.OpenTerminal(cfg, opt, false) // ensures the image itself
	}
	if err := svc.ensureImage(cfg); err != nil {
		return err
	}
	enf, err := svc.enforcer(cfg)
	if err != nil {
		return err
	}
	steps := enf.Prepare(cfg, opt)
	for _, cmd := range steps {
		if err := svc.runtime.Exec(cmd); err != nil {
			return errors.Join(fmt.Errorf("launch %s (%s): %w", cfg.App.Name, cmd.Desc, err), svc.teardown(cfg, len(steps) > 0))
		}
	}
	appArgs, err := svc.runtime.AppRunArgs(cfg, opt, enf.RunFlags(cfg))
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

// Stop tears a running app down via its enforcer's Teardown (the pod and its
// filtered netns for a pasta app, the container otherwise). Output is captured, so
// it is safe to call from the TUI.
func (svc Service) Stop(cfg domain.AppConfig) error {
	enf, err := svc.enforcer(cfg)
	if err != nil {
		return err
	}
	return svc.runtime.Exec(ports.Command{Args: enf.Teardown(cfg), Desc: "stop " + cfg.App.Name})
}

// teardown removes a half-built netns after a failed launch (fail-closed). It only
// fires when the enforcer had pre-steps to undo (pasta); for none/container there is
// nothing half-built. It returns any error so a failed/leaked teardown is visible to
// the caller (joined into the launch error) rather than silently swallowed.
func (svc Service) teardown(cfg domain.AppConfig, hadSteps bool) error {
	if !hadSteps {
		return nil
	}
	enf, err := svc.enforcer(cfg)
	if err != nil {
		return err
	}
	return svc.runtime.Exec(ports.Command{Args: enf.Teardown(cfg), Desc: "teardown " + cfg.App.Name})
}

// Build force-rebuilds an app's derived image (the explicit-rebuild path).
func (svc Service) Build(cfg domain.AppConfig) error { return svc.builder.Build(cfg) }

// ensureImage builds the derived image when app.install is set and the live image is
// missing or stale (its fingerprint label differs) — the auto-on-run trigger (§9.1).
func (svc Service) ensureImage(cfg domain.AppConfig) error {
	if !domain.HasInstall(cfg) {
		return nil
	}
	want := domain.BuildFingerprint(cfg)
	if got, err := svc.builder.Fingerprint(domain.DerivedImageRef(cfg.App.Name)); err == nil && got == want {
		return nil // already current
	}
	return svc.builder.Build(cfg)
}

// --- thin passthroughs so the front-ends drive everything through one facade ---

func (svc Service) List() ([]string, error)                    { return svc.store.List() }
func (svc Service) Load(name string) (domain.AppConfig, error) { return svc.store.Load(name) }
func (svc Service) Save(cfg domain.AppConfig) error            { return svc.store.Save(cfg) }
func (svc Service) Delete(name string) error                   { return svc.store.Delete(name) }
func (svc Service) Exists(name string) bool                    { return svc.store.Exists(name) }
func (svc Service) Path(name string) string                    { return svc.store.Path(name) }

func (svc Service) Marshal(cfg domain.AppConfig) ([]byte, error)   { return svc.store.Marshal(cfg) }
func (svc Service) LoadFile(path string) (domain.AppConfig, error) { return svc.store.LoadFile(path) }

func (svc Service) Search(term string) ([]ports.Result, error) { return svc.resolver.Search(term) }
func (svc Service) Resolve(ref string) (string, error)         { return svc.resolver.Resolve(ref) }

func (svc Service) Running() (map[string]bool, error)          { return svc.runtime.Running() }
func (svc Service) Logs(name string, tail int) (string, error) { return svc.runtime.Logs(name, tail) }

// Do runs a user-facing runtime command (restart/inspect/logs passthrough) with the
// host's stdio — for the CLI, where streaming output is wanted.
func (svc Service) Do(args []string) error { return svc.runtime.Do(args) }

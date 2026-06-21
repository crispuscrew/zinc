// Package ports declares the contracts between HyprZinc's application core
// (core/app) and the outside world (core/adapters/*). It is the hexagon's boundary:
// the app layer depends only on these interfaces, never on a concrete podman/nft/fs
// implementation, so a mechanism can be swapped by writing a new adapter — the
// motivating case being egress enforcement (NetEnforcer), where "not pasta" later
// is one more adapter, not a cross-cutting edit (docs/architecture.md §5.3, §13).
//
// ports depends only on core/domain (pure types); it performs no I/O itself.
package ports

import "github.com/crispuscrew/hyprzinc/core/domain"

// Command is one runtime instruction — the args passed to the container runtime,
// with optional stdin (used to pipe the nft ruleset into the lock-down step) and a
// short human label for dry-run output. It is the neutral unit a NetEnforcer
// emits and a Runtime executes, so neither side hardcodes the other's CLI.
type Command struct {
	Args  []string // arguments to the runtime (e.g. podman)
	Stdin string   // optional stdin
	Desc  string   // short human label (shown in dry-run)
}

// Result is one image-registry search hit.
type Result struct {
	Name        string
	Description string
}

// Store persists app definitions and provides the TOML codec for the editor
// round-trip (Marshal a draft, LoadFile it back). Adapter: core/adapters/fs.
type Store interface {
	List() ([]string, error)
	Load(name string) (domain.AppConfig, error)
	Save(cfg domain.AppConfig) error
	Delete(name string) error
	Exists(name string) bool
	Path(name string) string
	Marshal(cfg domain.AppConfig) ([]byte, error)   // encode a draft to TOML (for $EDITOR)
	LoadFile(path string) (domain.AppConfig, error) // decode an arbitrary .toml path (CLI path arg, editor round-trip)
}

// Runtime drives the container engine. Adapter: core/adapters/podman. AppRunArgs is
// pure (builds argv, no I/O) so plans can be inspected/dry-run; everything else
// performs I/O. netFlags are supplied by a NetEnforcer, so the runtime never knows
// which egress mechanism is in play.
type Runtime interface {
	AppRunArgs(cfg domain.AppConfig, opt domain.HostOptions, netFlags []string) ([]string, error)
	Exec(cmd Command) error // run one prepared command (pod create / nft / holder); capture output on failure
	// StartApp starts the app container detached (Setsid), terminal-wrapped if
	// app.terminal. It returns once the process is forked, before `podman run`
	// succeeds; onFail is invoked from the reaping goroutine if the app exits with an
	// error, so a post-fork failure can tear down the prepared (still-filtered) netns.
	StartApp(cfg domain.AppConfig, opt domain.HostOptions, runArgs []string, onFail func()) error
	OpenSession(app string, cmd []string, opt domain.HostOptions) error // blocking `exec -it` into a holder, in a terminal window (multiterminal)
	Exists(name string) bool                                            // does a container with this name exist (running or not)?
	Do(args []string) error                                             // user-facing passthrough (stop/restart/inspect/logs) with host stdio
	Running() (map[string]bool, error)                                  // names the runtime reports as running (list view)
	Logs(name string, tail int) (string, error)                         // last N log lines (logs view)
}

// ImageBuilder builds an app's derived image (FROM app.image + the install layer).
// Adapter: core/adapters/podman.
type ImageBuilder interface {
	Build(cfg domain.AppConfig) error       // force a build
	Fingerprint(ref string) (string, error) // read the build label; error if the image is absent
}

// ImageResolver discovers images and pins tags to digests (§5.5). Adapter:
// core/adapters/podman.
type ImageResolver interface {
	Search(term string) ([]Result, error)
	Resolve(ref string) (string, error)
}

// NetEnforcer establishes and enforces an app's network egress — THE swap point.
// Adapters today (core/adapters/netenforce): none, pasta (pod + nft), container. A
// future mechanism is one more implementation; the app layer is agnostic.
type NetEnforcer interface {
	RunFlags(cfg domain.AppConfig) []string                         // app container network attach (--pod … / --network …)
	Prepare(cfg domain.AppConfig, opt domain.HostOptions) []Command // steps to establish + LOCK the netns before the app starts
	Teardown(cfg domain.AppConfig) []string                         // tear it all down (pod rm / stop)
}

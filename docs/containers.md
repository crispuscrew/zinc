# Container runtime

`zcr` uses rootless Podman. Namespaces and explicit mounts are the main container
boundary; containers still share the host kernel. The managed baseline includes
`--cap-drop all`, `no-new-privileges`, and `--pull never`. Raw backend flags can
override that baseline and are warned.

Shared validation runs before launch; host-dependent checks additionally verify
mount sources, broker resources and [network provisioning](network-provisioning.md).
A successful schema check is not evidence that an image or service exists.

## Lifecycle

`zcr run APP` prints a plan; `--exec` launches. A nonempty
`StartConditions.Entrypoint` is `/bin/sh -c` inside the image. Empty ordinary
entrypoints preserve the image default. `EntrypointEnv` is a literal string map,
not host-shell expansion; reserved broker/runtime environment keys are refused.

`Terminal` opens a configured host terminal with an interactive TTY. Set
`ZINC_TERMINAL`, falling back to `TERMINAL`; a missing terminal command fails
with an actionable error. Ordinary foreground commands, detached Background
apps and terminal sessions follow different Podman lifecycle paths.

- `StopConditions.KeepAlive` retains the container instead of selecting `--rm`;
  it does not make an exited process continue executing.
- `StopConditions.Background` selects detached execution, or keeps an Attached
  holder alive after its last terminal closes.
- `StopConditions.Autorestart` uses Podman's `on-failure` and does not pair with
  `--rm`. A clean exit or manual stop is not an automatic failure restart.
- `StartConditions.ReadOnlyRootfs` uses `--read-only`. Backend-managed writable
  temporary locations and explicitly granted writable mounts remain separate.

## Attached sessions

`StartConditions.Attached` requires `Terminal` and an explicit `Entrypoint` or
`AttachedEntrypoint`. A detached holder under `--init` keeps one shared container
available; every terminal executes the selected command with `podman exec -it`.
`run` on a live attached app opens another session. `zcr term APP --shell` opens
a shell rather than the configured attached command.

`AttachedEntrypoint` overrides `Entrypoint` for sessions. Session environment is
`EntrypointEnv` overlaid by `AttachedEnv`, with the latter winning duplicate
names. This runtime overlay differs from inheritance, where each explicitly
authored environment map replaces its entire parent map.

Filesystem locks serialize holder creation and liveness markers under runtime
state. Each terminal waiter owns a lock, released by process death; the last
waiter removes the holder unless Background retains it. There is no resident
central daemon. The holder clears the image entrypoint so it does not
accidentally execute the app instead of holding sessions.

## Dependencies and resources

`DependsOn` starts missing container dependencies depth-first and leaves already
running ones alone. Cycles fail. It is runtime ordering, not an HTTP, database,
tunnel or DNS readiness check. The canonical schema has no `ReadyCheck` or
`ReadyTimeoutSec`; nonzero legacy probes cannot migrate losslessly.

`MaxCPUCores` supports fractional CPU quota; `MaxRamMiB` and `PIDsLimit` bound
memory and processes. Zero means no explicit limit in that field. There is no
canonical swap limit or VRAM limit. `UseNonRootUser` selects an existing image
account; it does not create one. `KeepUserID` requests host UID mapping and is
required for a filtered D-Bus grant.

`MinimizeFingerprint` removes Podman's `container` environment marker and asks
for hostname `localhost` at the owning container/pod UTS layer. This is best
effort, not a claim that software cannot detect containment.

## Preparation and inspection

Orchestration loads/resolves intent, validates, starts dependencies, builds a
derived image if needed, prepares network and desktop brokers, then launches.
Broker sockets must exist before bind mounts. Failures unwind owned resources;
network rollback stays default-deny.

Plans print shell-quoted managed arguments and quoted stdin content. Planned
audio sockets are placeholders: a real broker must be ready before execution.
Network domain rules may perform configured DNS lookups during planning.

`ps`, `inspect`, `logs`, `where`, `bus` and `net` report different aspects of the
running app. `where` exposes per-instance paths; `bus` reports proxy attribution;
`net` reads observed attachment and counters. None is a blanket attestation
against raw flag overrides. Full command syntax is in the
[runner README](../container/runner/README.md).

# Container runtime

`zcr` uses rootless Podman: namespaces/explicit mounts, shared host kernel,
`--cap-drop all`, `no-new-privileges`, `--pull never`. Raw flags warn and can override this.
Validation cannot prove image/service availability; launch also checks mounts,
brokers and [network provisioning](network-provisioning.md).

## Lifecycle

`zcr run APP` plans; `--exec` launches. Nonempty `StartConditions.Entrypoint` uses
`/bin/sh -c` inside the image; empty preserves its default. `EntrypointEnv` is
literal, without host-shell expansion; reserved broker/runtime keys are refused.
`Terminal` needs an installed `ZINC_TERMINAL` or fallback `TERMINAL`, opens an
interactive TTY, and fails if unavailable. Foreground, detached and terminal paths differ.

| Setting | Effect |
| --- | --- |
| `StopConditions.KeepAlive` | Retain container, omit `--rm`; cannot keep an exited process executing |
| `StopConditions.Background` | Detach, or retain Attached holder after last terminal closes |
| `StopConditions.Autorestart` | `on-failure`, never `--rm`; no restart on clean exit/manual stop |
| `StartConditions.ReadOnlyRootfs` | `--read-only`; backend temporary storage and writable mounts remain separate |

## Attached sessions

- `StartConditions.Attached` requires `Terminal` and explicit `Entrypoint` or
  `AttachedEntrypoint`. A detached `--init` holder clears the image entrypoint.
- Terminals use `podman exec -it`; another `run` opens another session.
  `zcr term APP --shell` substitutes a shell for the attached command.
- `AttachedEntrypoint` overrides `Entrypoint`; `AttachedEnv` overlays `EntrypointEnv`,
  winning duplicates. [Inheritance](schema.md#inheritance-is-a-yaml-operation) replaces whole maps.
- Runtime filesystem locks serialize holder creation/liveness. Each waiter owns a
  death-released lock; the last removes the holder unless Background retains it.
  No central daemon is required.

## Dependencies and resources

`DependsOn` starts missing dependencies depth-first, preserves running ones and
rejects cycles. It checks ordering, not HTTP/database/tunnel/DNS readiness.
No `ReadyCheck`/`ReadyTimeoutSec`; nonzero legacy probes cannot migrate losslessly.

`MaxCPUCores` allows fractions; `MaxRamMiB`/`PIDsLimit` bound memory/processes.
Zero means no explicit limit; no swap/VRAM field exists. `UseNonRootUser` selects
an existing image account; `KeepUserID` maps host UID and is required for filtered D-Bus.
`MinimizeFingerprint` removes the `container` env marker and requests `localhost`
at the container/pod UTS owner; containment remains detectable.

## Preparation and inspection

Order: resolve/validate, dependencies, derived build, network/desktop brokers, launch.
Sockets precede mounts; failure unwinds owned resources and restores default-deny networking.
Plans quote managed argv/stdin, use placeholder audio sockets and may query configured DNS.
Real brokers must be ready at execution.

[Commands](../container/runner/README.md#commands): `ps`, `inspect`, `logs`, `where`
(instance paths), `bus` (proxy attribution), `net` (attachment/counters).
Inspection cannot attest protection against raw overrides.

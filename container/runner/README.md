# zcr - Zinc Container Runner

`zcr` launches rootless Podman containers from schema v4 app files in
`$XDG_CONFIG_HOME/zinc/apps` (default `~/.config`). `zc` and launchers delegate here.

## Commands

```text
zcr run <app[@instance]> [--instance NAME] [--exec] [-v HOST:CONTAINER[:OPTIONS]]...
zcr build <app>
zcr validate <app>
zcr stop|restart|inspect <app>
zcr logs <app> [-f]
zcr term <app> [--shell]
zcr ps
zcr where <app[@instance]> [--json]
zcr bus [--json]
zcr net [app[@instance]] [--json]
zcr recheck <app>
zcr image search <term> | resolve <ref>
```

App arguments also accept YAML paths. `run` prints a shell-quoted plan with quoted
build/network stdin; `--exec` launches. Planned PipeWire mounts are placeholders:
launch must prepare the restricted socket first.

## Lifecycle and backend options

- Entrypoints use `/bin/sh -c`; an empty ordinary entrypoint keeps the image default.
- `Attached` requires `Terminal`. Reopening shares a holder; the last terminal removes
  it unless `Background` is set. `AttachedEnv` overlays `EntrypointEnv`; inheritance
  replaces whole env maps. See [lifecycle details](../../docs/containers.md).
- `ReadOnlyRootfs` enables `--read-only`; `Autorestart` uses `on-failure`, never `--rm`.
- `CreatorFlags` are `podman build` argv, fingerprinted and sufficient to trigger a build.
  `RunnerFlags` precede the image in `podman run`. Only NUL is rejected; both warn,
  can override containment, and are never passed to helpers.
- `MinimizeFingerprint` removes the `container` marker and requests hostname `localhost`;
  it is best effort and retains capability drops, seccomp and no-new-privileges.

[Migration](../../docs/migration.md) rejects unrepresentable nonzero readiness,
swap, capability and tunnel settings rather than discarding them.

## Network policy

Empty `NetworkMeta.Interfaces` means `--network none`. Otherwise a matching
[owner-provisioned manifest](../../common/adapters/network/README.md) is mandatory:
rules create no topology. Ordered policy is default-deny; DNS has no host fallback.

`zcr net` reports observed attachment; `zcr net <app> --json` returns counters.
`rule[0]` is a canonical rule index; reciprocal policy may reject at either endpoint.

## Desktop access

- [Audio](../../docs/audio.md): directional default/named PipeWire grants require
  owner-deployed WirePlumber policy and a private socket, never a raw-session fallback.
  ALSA paths must be real, direction-correct character devices, not all of `/dev/snd`.
- [D-Bus](../../docs/desktop-access.md): `Talk`/`Own` require `KeepUserID`. The filtered
  proxy stays outside the pod; `bus --json` attributes PIDs and `where` reports instance paths.

## Build and layout

```sh
make build            # pinned Podman build
make check            # formatting, vet, tests
make netfilter-image  # local network/D-Bus helper
```

See [architecture](../../docs/architecture.md) for the package layout.

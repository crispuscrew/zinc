# zcr - Zinc Container Runner

`zcr` reads canonical schema v4 app files from `~/.config/zinc/apps` and launches
rootless Podman containers. `zc` and the launchers delegate to this binary.

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

An app argument may also be a YAML file path. `run` prints a shell-quoted plan by
default; `--exec` launches it. Plans use quoted here-documents for build and network
stdin. A planned PipeWire mount is a placeholder: a real launch must establish its
restricted socket before starting the app.

## Lifecycle and backend options

- Presentation fields live under `LauncherMeta`.
- `StartConditions.Entrypoint` and `AttachedEntrypoint` use `/bin/sh -c` grammar
  inside the image. An empty ordinary entrypoint preserves the image default.
- `StartConditions.Attached` requires `Terminal` and uses a detached holder with
  interactive sessions. `run` on a live attached app opens another session.
- Every session receives `EntrypointEnv` overlaid by `AttachedEnv`, with the latter
  winning duplicate keys. Environment argv is sorted. Inheritance replaces an
  explicitly supplied env map rather than merging it with the parent's map.
- Closing the last attached terminal removes the holder unless
  `StopConditions.Background` keeps it alive.
- `StartConditions.ReadOnlyRootfs` enables Podman's read-only root filesystem.
- `StopConditions.Autorestart` uses `on-failure` and never pairs with `--rm`.
- `CreatorFlags` are direct `podman build` argv and participate in the derived image
  fingerprint. Flags alone also trigger a derived build, without an empty RUN layer.
- `RunnerFlags` are direct `podman run` argv immediately before the image. They can
  override structured containment. Only NUL is rejected; warnings appear during
  planning and actual launches. Neither flag list is sent to helper containers.
- `MinimizeFingerprint` removes Podman's `container` env marker and requests
  `localhost` as hostname at the owning container/pod UTS layer. This is best effort;
  capability drops, seccomp defaults and no-new-privileges remain in place.

Legacy files pass through migration. Nonzero readiness checks/timeouts, swap limits,
capability lists and tunnel settings that cannot be represented produce migration
errors. There is no silent discard or compatibility bypass.

## Network policy

`NetworkMeta.Interfaces` declares Zinc interface IDs. `RulesByPriority` carries
ordered `From`/`To` peers, endpoint filters, `Domains`, `Protocols` and
`AllowAllExcept`. The network adapter enforces default-deny policy in an
owner-provisioned, packet-preserving namespace before the app joins it.

No interfaces means `--network none`. Networked apps require a matching provisioned
manifest; a schema rule does not create host topology. DNS uses
`DNS.ResolversByPriority`; domain resolution cannot fall back to host DNS.
See the shared network adapter for the provisioning contract.

`zcr net` reports observed pod attachment and `zcr net <app> --json` reports the
enforcer's counters. `rule[0]` refers to a canonical rule index. Reciprocal app policy
can reject traffic at either endpoint.

## Desktop access

`AudioMeta.Playback`, `Microphone` and `Monitor` use `PipeWireDefault`, exact
`PipeWireDevices` selectors and `ALSADevices`. PipeWire requires a restricted per-app
socket, with no raw session fallback. ALSA grants must name existing host character
devices in the correct playback/capture direction; the whole `/dev/snd` is not granted.

`DBusMeta.Talk` and `Own` grant a filtered session bus. `InternalUserMeta.KeepUserID`
is required. The proxy stays outside the app pod and exposes its own socket rather
than the desktop's raw bus. `zcr bus --json` attributes proxy PIDs to apps;
`zcr where` reports the corresponding per-instance state and socket paths.

## Build and layout

```sh
make build
make check
make netfilter-image
```

Builds and checks use the pinned Podman toolchain. `domain` contains pure policy,
`ports` the interfaces, `app` orchestration, `adapters` the mechanisms, and `wire`
the composition. CLI commands live in the runner's root package.

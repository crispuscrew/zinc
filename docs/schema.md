# Canonical app schema

The authoritative writing reference is [schema.go](../common/domain/schema/schema.go):
field names come from YAML tags; nested structs, enums and comments define the shape.
The [README samples](../README.md#write-an-app-definition) show basic YAML and inheritance.
This page explains behavior and backend limits rather than redefining fields.
Files: `$XDG_CONFIG_HOME/zinc/apps/<AppNameID>.yaml`; XDG default: `~/.config`.
`ZincContainer` and `ZincVirtualization` share fields, not universal backend support.
Stores [migrate](migration.md) before strict decoding; new examples must pass
`KnownFields(true)` without migration. Value and host checks follow; unsupported intent errors.

## Field map

| Fields | Semantics |
| --- | --- |
| `SchemaVersion`, `Type`, `AppNameID`, `Inherits` | Version, runtime, identity, optional parent |
| `LauncherMeta` | `Icon`, `Description`, `Group`: presentation only |
| `StartConditions`, `StopConditions`, `ResourcesMeta`, `InternalUserMeta`, `MinimizeFingerprint` | [Container lifecycle/limits](containers.md); [VM semantics and unsupported fields](virtualization.md) |
| `ImageMeta`, `Configs`, `Volumes`, `Keys`, `HostTheme` | [Images/files](images-and-mounts.md); [VM provisioning](virtualization.md#disks-and-firmware-state) |
| `DisplayMeta`, `DBusMeta`, `NotificationMeta` | [Desktop grants](desktop-access.md); [VM boot/display hardware](vm-hardware.md) |
| `NetworkMeta`, `AudioMeta` | [Packet policy](network-policy.md), [DNS](dns.md), [directional audio](audio.md) |
| `CreatorFlags`, `RunnerFlags` | Ordered raw argv; limits below |

App IDs start alphanumeric and use lowercase `[a-z0-9._-]`.
VM runtime-options IDs additionally have a 96-character limit.
Container resource zero means no explicit limit; VMs need positive RAM/whole CPUs.
No canonical swap limit, capability list, readiness probe, tunnel or `VirtualizationMeta`.

## Minimal offline container

See [zinc-hello.yaml](../common/examples/apps/zinc-hello.yaml) and the
[README examples](../README.md#write-an-app-definition).

More checked [examples](../common/examples/README.md): sessions, peers, DNS, audio, VM options.

## Inheritance is a YAML operation

`Inherits` resolves a store parent on every read; parent edits affect the next launch.
Audit with `zc validate <app> --resolved`:
- Omitted keys inherit; explicit false/zero/empty scalars override; lists replace.
  Mappings merge, except the whole-map replacements below.
- `EntrypointEnv`, `AttachedEnv` and each audio direction replace their whole mapping.
  `AudioMeta: {Microphone: {}}` revokes inherited microphone grants.
- Missing parents, cycles or depth >8 fail. IDs are checked before filesystem access;
  aliases resolve in their original document before merging.
- Stores separate raw/resolved reads to retain omission. Edit sparse YAML directly;
  creator form saves refuse inherited apps rather than flatten them.

## Raw argv is an explicit escape hatch

Each element is one argument: order/whitespace preserved, no shell splitting.
Nonempty lists warn: isolation, networking, devices, disks and lifecycle can be
overridden; typed fields alone cannot prove protection. Backend/topology checks still apply.
Container entrypoints instead use `/bin/sh -c`; VM guest commands/environments are rejected.

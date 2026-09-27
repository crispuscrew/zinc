# Canonical app schema

The declarations in [schema.go](../common/domain/schema/schema.go) define schema
version **4**. App files live at `$XDG_CONFIG_HOME/zinc/apps/<AppNameID>.yaml`
(default `~/.config`). `ZincContainer` and `ZincVirtualization` share this model;
sharing a field does not imply that both backends can implement it.

Stores migrate recognized legacy input before strict YAML decoding. New examples
must decode with `KnownFields(true)` without migration. A valid YAML document is
not necessarily a valid app: shared validation checks values, and launch adds
host-dependent checks. Unsupported intent is an error, not a silent no-op.

## Field map

| Field/group | Meaning |
| --- | --- |
| `SchemaVersion`, `Type`, `AppNameID`, `Inherits` | Version, runtime, identity, optional parent app |
| `LauncherMeta.Icon`, `.Description`, `.Group` | Picker presentation only |
| `StartConditions.DependsOn` | Start dependencies before this app; not service readiness |
| `StartConditions.Entrypoint`, `.EntrypointEnv` | Container command and environment |
| `StartConditions.Terminal`, `.Attached` | Host terminal; optional shared container sessions |
| `StartConditions.AttachedEntrypoint`, `.AttachedEnv` | Attached command/environment overrides |
| `StartConditions.ReadOnlyRootfs` | Container rootfs or VM root block device read-only |
| `StartConditions.LoaderBIOS`, `.SecureBoot`, `.TPM` | VM boot choices |
| `StopConditions.KeepAlive`, `.Background`, `.Autorestart` | Runtime-specific retention/restart behavior |
| `MinimizeFingerprint` | Best-effort container markers or VM branding, not anonymity |
| `ResourcesMeta.MaxCPUCores`, `.MaxRamMiB`, `.PIDsLimit` | Container quotas; VM sizing, with no guest PID limit |
| `InternalUserMeta` | `UseNonRootUser`, `NonRootUserName`, `KeepUserID` |
| `ImageMeta.Image`, `.Install`, `.SourceTag` | Image/base path, installation steps, source provenance |
| `ImageMeta.CloudInit`, `.PublicSSHKeyPath` | VM first-boot provisioning and public key |
| `DisplayMeta` | GPU denial, dimensions, security-context choices, Vulkan |
| `NetworkMeta.Interfaces`, `.RulesByPriority`, `.DNS` | [Packet policy](network-policy.md) and explicit resolvers |
| `AudioMeta.Playback`, `.Microphone`, `.Monitor` | [Directional device mappings](audio.md) |
| `DBusMeta`, `NotificationMeta` | [Filtered bus and notification policy](desktop-access.md) |
| `Configs`, `Volumes`, `Keys`, `HostTheme` | [Explicit filesystem grants](images-and-mounts.md) |
| `CreatorFlags`, `RunnerFlags` | Ordered raw backend argv |

App IDs start alphanumeric and contain lowercase `[a-z0-9._-]` (maximum 96
characters). Resource zero values mean no explicit container limit; VMs require
positive RAM and positive whole CPU counts. There is no canonical swap-limit,
capability-list, readiness-probe, tunnel or `VirtualizationMeta` field.

## Minimal offline container

```yaml
SchemaVersion: 4
Type: ZincContainer
AppNameID: shell
LauncherMeta:
  Description: Offline terminal
ImageMeta:
  Image: docker.io/library/alpine@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d
StartConditions:
  Entrypoint: /bin/sh
  Terminal: true
DisplayMeta:
  DisableGpuAccess: true
NetworkMeta:
  Interfaces: []
```

The [example directory](../common/examples/README.md) contains complete checked
fixtures for attached sessions, network peers, DNS, audio and external VM options.

## Inheritance is a YAML operation

`Inherits` names another app in the store. Resolution happens on every read, so
changing a parent changes its children at the next launch. Audit the result with
`zc validate <app> --resolved`.

- Omitted child keys inherit; explicit `false`, zero and empty values override.
- Nested mappings normally merge field by field. Lists replace, never append.
- `EntrypointEnv` and `AttachedEnv` each replace their whole inherited map.
- Each audio direction replaces its entire inherited mapping. For example,
  `AudioMeta: {Microphone: {}}` revokes that direction's inherited grants.
- Missing parents, cycles and chains deeper than eight fail resolution.
- Parent IDs are checked before filesystem access. YAML aliases are resolved in
  their original document before merging.

These rules depend on knowing which keys were authored; a decoded struct cannot
distinguish omission from a zero value. Stores keep raw and resolved reads
separate. The creator refuses form saves of inherited apps rather than flatten
their sparse YAML; edit the file instead.

## Raw argv is an explicit escape hatch

Each `CreatorFlags` or `RunnerFlags` element is one argument, preserving order
and whitespace; there is no shell splitting. Nonempty lists produce warnings.
They can override typed isolation, networking, devices, disks and lifecycle
controls. Do not infer effective protection from structured fields alone when
raw flags are present. Backend applicability and mandatory topology checks can
still reject a launch.

Container entrypoint strings are different: they use `/bin/sh -c` inside the
image. VM guest entrypoints and environment injection are unavailable without
a guest agent and are rejected. See [migration](migration.md) before converting
old files, and [VM limits](virtualization.md) before reusing container intent.

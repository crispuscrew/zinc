# Migrating existing app definitions

The current writer emits canonical schema v4. Readers recognize supported v3
fields and legacy aliases, including versionless inheritance fragments, before
strict decoding. A v4 version number alone does not prove canonical field use.
Other version numbers are not silently upgraded.

Migration occurs in memory; loading is not permission to rewrite a file. Saving
through the creator writes current intent. Review `zc validate APP --resolved`
before launching, especially for inherited apps.

## Recognized field moves

| Legacy location | Canonical location |
| --- | --- |
| `Icon`, `Description`, `Group` | `LauncherMeta` fields |
| `Env` | `StartConditions.EntrypointEnv` |
| `ReadOnlyRootfs` | `StartConditions.ReadOnlyRootfs` |
| `StartConditions.Multiterminal` | `StartConditions.Attached` |
| `StartConditions.MultiterminalEntrypoint` | `StartConditions.AttachedEntrypoint` |
| `StartConditions.MultiterminalEnv` | `StartConditions.AttachedEnv` |
| `StartConditions.Autorestart` | `StopConditions.Autorestart` |
| `VirtualizationMeta.MemoryMiB`, `.VCPUs` | `ResourcesMeta.MaxRamMiB`, `.MaxCPUCores` |
| VM display dimensions/Vulkan | `DisplayMeta` |
| VM SecureBoot/TPM/firmware choice | `StartConditions` boot fields |
| VM public SSH key | `ImageMeta.PublicSSHKeyPath` |

Legacy audio scalars/lists and representable DNS/network rules are translated
only by the recognized converters. Conflicting old and new values are errors.
Some generated v3 VM files contain zero container-resource placeholders; the
converter recognizes those when moving the actual VM sizes.

## Refusal is intentional

Nonzero removed settings cannot simply disappear: capability lists, swap limits,
readiness probes/timeouts, tunnels and unrepresentable network routing/publication
intent cause migration errors. Empty legacy residue may be removed. Unsupported
keys still fail strict decoding after migration.

Old VM disk pins and host hardware choices need an explicit move to
`$XDG_CONFIG_HOME/zinc/runtime/vm/<app>.json`. The schema migrator cannot safely
create that file or decide how to preserve a machine's disk/firmware identity.
Nonzero legacy `BaseDigest`, disk size, display/device profile, media, forwards
and other runtime-only values therefore block migration rather than being lost.
Review MAC and guest-account changes too; not every legacy field has a lossless
automatic transformation.

Use the [VM options format](virtualization.md) and
[checked paired example](../common/examples/README.md). Retain the authorized
base digest and hardware choices; calculating a fresh digest does not authorize
different content. If intent has no lossless transformation, stop and revise it
explicitly rather than deleting the failing field to silence validation.

## Networking and audio need deployment work

Converting YAML does not provision networking. Old automatic pasta namespaces,
bridges, `Via` routing and WireGuard setup are no longer the launch path.
Networked apps require [provisioned manifests](network-provisioning.md), exact
peer policy and a packet-preserving path.

Converting `Playback: default` to `Playback: {PipeWireDefault: true}` does not
deploy the audio policy. Brokered PipeWire requires the host owner's explicit
[WirePlumber deployment](../integration/wireplumber/README.md); missing policy
fails preparation. There is no raw-session-socket compatibility fallback.

Historical release notes describe what their releases did. They are not the
current schema reference; use [schema.md](schema.md) for new definitions.

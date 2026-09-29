# Migrating existing app definitions

Writers emit schema v4. Readers migrate supported v3/legacy aliases, including
versionless inheritance fragments, before strict decoding. A v4 label alone is
not canonical; other versions are not silently upgraded.
Loading migrates in memory; creator saves write current intent. Review
`zc validate APP --resolved` before launch, especially with inheritance.

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

Recognized converters handle legacy audio scalars/lists and representable DNS/network
rules. Old/new conflicts error; generated v3 VM zero-resource placeholders are
recognized when moving actual VM sizes.

## Refusal is intentional

- Nonzero capability lists, swap limits, readiness probes/timeouts, tunnels and
  unrepresentable routing/publications error. Empty legacy residue may disappear;
  remaining unsupported keys fail strict decoding.
- Explicitly move VM pins/hardware to `$XDG_CONFIG_HOME/zinc/runtime/vm/<app>.json`.
  Nonzero legacy `BaseDigest`, disk size, display/device profile, media, forwards
  and other runtime-only values block migration; it cannot safely create that file.
  Review MAC/guest-account changes and preserve disk/firmware identity.
- Follow [VM options](virtualization.md) and the [paired example](../common/examples/README.md).
  Retain authorized pins/hardware: a fresh digest does not authorize different bytes.
  Revise untranslatable intent explicitly; do not delete fields just to pass validation.

## Networking and audio need deployment work

- Networking needs [manifests](network-provisioning.md), exact peer policy and
  packet-preserving paths; automatic pasta/bridge/`Via`/WireGuard setup is gone.
- `Playback: default` becomes `Playback: {PipeWireDefault: true}`, but still needs
  owner-approved [WirePlumber deployment](../integration/wireplumber/README.md).
  Missing policy fails preparation; no raw-session-socket fallback exists.

Use the current [schema](schema.md), not historical release notes, for new definitions.

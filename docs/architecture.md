# Zinc architecture

This index covers the schema v4 checkout. For earlier behavior, use the
[changelog](../CHANGELOG.md); to start, use the [quickstart](quickstart.md).

## Design and boundaries

- App intent is YAML; host-local VM options are JSON. Tools delegate to runners.
- Pure validation checks intent; adapters check host resources, topology, pins,
  brokers and backend support. Plans cannot attest availability or replace preparation.
- Declared NICs require owner-provisioned, packet-preserving, default-deny networking.
  No automatic pasta/bridge/Via/WireGuard provisioner replaces the historical setup.
- Desktop brokers prepare outside the app; PipeWire requires deployed Zinc policy.
- Raw backend argv warns because it can override typed guarantees.

## Focused reference

| Task | Reference |
| --- | --- |
| Author or upgrade | [Schema](schema.md), [migration](migration.md), [examples](../common/examples/README.md) |
| Choose tools / understand layout | [Components](components.md) |
| Run containers / grant files | [Containers](containers.md), [images and mounts](images-and-mounts.md) |
| Grant display, bus or sound | [Desktop access](desktop-access.md), [audio](audio.md) |
| Connect apps | [Policy](network-policy.md), [provisioning](network-provisioning.md), [DNS](dns.md) |
| Run or install VMs | [Virtualization](virtualization.md), [hardware](vm-hardware.md) |
| Build, test or package | [Builds and checks](build-and-checks.md) |

Command references: [zc](../creator/README.md), [zcr](../container/runner/README.md),
[zvr](../virtualization/runner/README.md). Both runners expose the shared DNS worker
and authenticate its manifest-bound readiness.

## Earlier section references

Numbered references in older comments map to these current contracts:

| Former section | Current reference |
| --- | --- |
| 1-2: principles and stack | This index; [components](components.md) |
| 3, 3.1: app config and inheritance | [Schema](schema.md), [migration](migration.md) |
| 4, 9: tools and creator/runner split | [Components](components.md), tool READMEs |
| 5.1: container isolation | [Containers](containers.md) |
| 5.2, 5.4, 5.6-5.8: desktop, GPU, theme, bus | [Desktop access](desktop-access.md), [mounts](images-and-mounts.md) |
| 5.3, 6.1-6.5: network model and enforcement | [Policy](network-policy.md), [provisioning](network-provisioning.md), [DNS](dns.md) |
| 5.5, 7: image trust and derived builds | [Images and mounts](images-and-mounts.md) |
| 5.9: audio | [Audio](audio.md) |
| 6.6: dependency ordering | [Containers](containers.md), [virtualization](virtualization.md) |
| 8: build/release | [Build and checks](build-and-checks.md) |
| 10: guests | [Virtualization](virtualization.md), [VM hardware](vm-hardware.md) |
| 11-13: host surface, desktop integration, layout | [Components](components.md), [provisioning](network-provisioning.md) |
| 14-15: tradeoffs/status | Limits on each focused page; [release history](../CHANGELOG.md) |

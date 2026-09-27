# Zinc architecture

**Priority: Stable, then Secure, then Beautiful.** Zinc authors and runs Linux
desktop applications through rootless Podman or QEMU. This index describes the
current canonical schema v4 checkout, not the behavior of earlier releases.
Historical release behavior remains in [CHANGELOG.md](../CHANGELOG.md).

## Design and boundaries

- App intent is YAML; host-local VM hardware choices are separate JSON.
- Shared validation is pure. Runtime adapters additionally check host resources,
  provisioned topology, disk pins, broker readiness and backend applicability.
- The creator and launchers execute runner binaries rather than import runners.
- Networking is default-deny and needs an owner-provisioned packet-preserving
  topology whenever an interface is declared. There is no automatic pasta setup.
- Brokered desktop access is prepared outside the application before launch.
  PipeWire needs an explicitly deployed Zinc WirePlumber policy.
- Raw backend argv can override typed guarantees and produces warnings.
- Plans expose managed intent; they do not replace launch-time preparation or
  attest that host services, firmware, images and devices are available.

## Focused reference

| Page | Contents |
| --- | --- |
| [Schema](schema.md) | Canonical fields, inheritance, strict decoding, raw flags |
| [Migration](migration.md) | Recognized aliases, lossless conversion, refusal cases |
| [Components](components.md) | Tool boundaries, ports/adapters, repository layout, launchers |
| [Containers](containers.md) | Lifecycle, dependencies, terminal holders, resource controls |
| [Images and mounts](images-and-mounts.md) | Pins, derived builds, config bundles, volumes, keys |
| [Desktop access](desktop-access.md) | Wayland identity, GPU limits, bus attribution, notifications |
| [Network policy](network-policy.md) | Ordered From/To rules, endpoint filters, peer consent, counters |
| [Network provisioning](network-provisioning.md) | Manifest trust, namespaces, TAPs, publications, startup ordering |
| [DNS](dns.md) | Five transports, proxy commands, authenticated readiness, IP snapshots |
| [Audio](audio.md) | Directional grants, WirePlumber deployment, revocation, ALSA |
| [Virtualization](virtualization.md) | External options, disks, supervision, host-only limits |
| [VM hardware](vm-hardware.md) | Display profiles, Windows installation, firmware, TPM, identity |
| [Build and checks](build-and-checks.md) | Go pins, vendors, CI, Nix compatibility, integration suites |

Start with the [quickstart](quickstart.md) or the tested
[canonical examples](../common/examples/README.md). Command details live with
[zc](../creator/README.md), [zcr](../container/runner/README.md), and
[zvr](../virtualization/runner/README.md).

## Earlier section references

Older code comments use numbered sections from the former monolithic document.
This map retains those reference meanings without presenting obsolete behavior
as a current contract.

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

The old automatic bridge/Via/WireGuard architecture is historical. Its removal
does not imply a replacement provisioner ships with this checkout. The shared
DNS worker is available through both runners, with authenticated manifest
readiness checks; see [DNS integration](dns.md).

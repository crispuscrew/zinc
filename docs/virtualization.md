# VM runtime and host-only limits

`zvr` runs `qemu-system-x86_64` directly in the user session, with local display.
Zinc owns supervision/disks/firmware/cleanup; managed-save/snapshots are not implied.

## App intent and external VM options

[App YAML](schema.md) is separate from `$XDG_CONFIG_HOME/zinc/runtime/vm/<app>.json`
(default `~/.config/...`). [Version-1 wire type](../common/domain/vmoptions/config.go):
- `Version`, `AppNameID`, `Image` bind options to intent; `BaseDigest` requires an
  authorized `sha256:<64 lowercase hex>` pin; `DiskSizeGiB` cannot resize existing state.
- `Display`/`Devices` choose [hardware](vm-hardware.md); discs/port mappings use `InstallMedia`/`ForwardPorts`.
- `zc new --vm` creates both files, refusing existing destinations. `--runtime-options`
  selects another JSON; [CLI overrides](../virtualization/runner/README.md#runtime-options)
  last one invocation, replace media/forward lists and never edit files.
  YAML-path launches need explicit options or a base pin.
- `zvr pin /absolute/base.qcow2` calculates, never authorizes. Review bytes separately;
  launch never approves changes by repinning. See the [paired example](../common/examples/README.md).

## Disks and firmware state

Instance qcow2 overlays: `$XDG_DATA_HOME/zinc/vms`; bases stay read-only. Run verifies
backing path/format/size, never recreates/resizes existing overlays. Hash caches bind
device/inode/size/nanosecond timestamps, not an owner able to rewrite image and cache.
Manifests bind pin/devices/firmware against silent reinterpretation.
`reset --confirm` deletes overlay/NVRAM/TPM, keeps bases/options, refuses live supervisors.
Creator YAML deletion keeps disks/options; VM rename errors because YAML alone cannot move identity.

`ReadOnlyRootfs` attaches root block read-only, not snapshot mode; guest support required.
Writable installation/cloud-init is forbidden. `ImageMeta.CloudInit` provisions first boot
with `PublicSSHKeyPath` and explicit guest account, not per-launch Install.
No exact host UID/GID mapping, implicit sudo or passwords.

## Supervision

Detached supervisors receive immutable snapshots over private pipes, then confirm guest/QMP
readiness before acknowledgement. [Lifecycle](../virtualization/runner/README.md#supervision-and-lifecycle) defines
crash-only `Autorestart` backoffs/limits/reset and intent/pin/topology/host revalidation.
Persisted generation-specific stop cancels retries; clean exits/preparation failures/uncertain cleanup never restart.
Stop uses QMP ACPI then signal escalation; live PID identity is checked, not just a recycled PID.
`DependsOn` checks the graph/starts missing dependencies through runners, not guest health.
Supervisor owns TPM/audio lifetimes; audio loss reports without killing guests or discarding storage.

## Shared fields stop at the guest boundary

**No guest agent:** guest entrypoints/environments, Attached commands, filesystem/config
sharing, private keys, themes, bus/notifications, exact host UID, required compositor
security context and guest PID limits are rejected.
`Terminal` opens host terminal/serial; `KeepAlive` uses `-no-shutdown` after guest shutdown.
Graphical Background is refused: GTK closure terminates QEMU; no detachable frontend.
Serial Terminal + Background preserves the guest after terminal closure.
No interfaces means no managed NIC; otherwise [pre-created TAPs/filtering](network-provisioning.md)
and configured [DNS readiness](dns.md) are required. No default slirp NAT/pasta fallback.

## Plans and raw flags

Plans may query configured DNS, but write no state, start no helpers or open audio devices.
Broker endpoints are placeholders; raw values are omitted as potential secrets.
`RunnerFlags` append QEMU argv; `CreatorFlags` target installers (`zvr install --app APP`).
Raw flags warn of containment/disk/lifecycle/offline overrides; plans cannot replace preparation.

# VM runtime and host-only limits

`zvr` drives `qemu-system-x86_64` directly from the user session. This keeps an
interactive local display close to the guest renderer rather than requiring
libvirt plus a remote-display viewer. The cost is that Zinc owns supervision,
disk state, firmware and cleanup; managed-save/snapshot features are not implied.

## App intent and external VM options

App YAML uses the [shared schema](schema.md). Host-local hardware and disk pins
live at `$XDG_CONFIG_HOME/zinc/runtime/vm/<app>.json`, default `~/.config/...`.
The [vmoptions.Config](../common/domain/vmoptions/config.go) format is version 1:

| JSON field | Purpose |
| --- | --- |
| `Version`, `AppNameID`, `Image` | Bind options to the format and authored app/image |
| `BaseDigest` | Required authorized `sha256:<64 lowercase hex>` disk pin |
| `DiskSizeGiB` | Overlay size choice, not permission to resize existing state |
| `Display` | None, Window, Accelerated, Compatible, or automatic when empty |
| `Devices` | Virtio or Compatible hardware profile |
| `InstallMedia` | Explicit installation/driver discs |
| `ForwardPorts` | Protocol, BindAddress, HostPort, GuestPort, logical Interface |

`zc new --vm` creates YAML and JSON together and refuses existing destinations.
`--runtime-options` selects a different JSON file. Runner CLI overrides are
one-invocation choices, not file edits; media/forward lists replace rather than
append. Running YAML by path requires explicit options or an explicit base pin.

`zvr pin /absolute/base.qcow2` calculates a candidate digest. Authorization is a
separate review: launch never blesses changed bytes by calculating a new pin.
See the [paired example](../common/examples/README.md) and
[runner command reference](../virtualization/runner/README.md).

## Disks and firmware state

Each instance uses a qcow2 overlay beneath `$XDG_DATA_HOME/zinc/vms`; the base
remains read-only. Run verifies the backing path, format and size rather than
recreating or resizing existing overlays. Base hashes are cached against file
identity including device, inode, size and nanosecond timestamps; this does not
protect against an owner who can rewrite both image and cache.

Instance manifests bind disk pin, devices and firmware so a later edit cannot
silently reinterpret existing state. `reset --confirm` explicitly deletes guest
overlay/NVRAM/TPM state while keeping bases and runtime options, and refuses a
live supervisor. Deleting app YAML through the creator keeps disks/options;
VM rename is refused because identity cannot safely move by renaming YAML alone.

`ReadOnlyRootfs` attaches the root block device read-only, not QEMU snapshot mode.
The guest OS must support that mode; writable installation/cloud-init cannot use
it. `ImageMeta.CloudInit` enables first-boot provisioning, with a public SSH key
and explicit guest user selection. Install steps are not commands replayed on
every launch. Exact host UID/GID mapping, implicit sudo and passwords are not
provided.

## Supervision

A detached supervisor owns each instance and receives an immutable launch
snapshot over private pipes. It confirms guest/QMP readiness before launcher
acknowledgement. PID identity is checked against live process state before
signalling; a recycled PID is not sufficient identity. Graceful stop uses QMP's
ACPI power button, with signal escalation if needed.

`Autorestart` handles unexpected nonzero QEMU exits using 1, 2, 4, 8 and 16
second backoffs, at most five retries per burst. Five stable minutes reset the
burst. Clean exit, explicit stop, preparation failure or uncertain cleanup do
not restart. Restart revalidates intent, disk pins, provisioning and host support.
Persisted generation-specific stop intent also cancels pending restarts.

`DependsOn` checks the dependency graph and starts missing dependencies through
their runners. This is process ordering, not guest OS/application health. The
supervisor owns audio-holder and TPM lifetimes; audio revocation is reported
without killing the guest or discarding its storage.

## Shared fields stop at the guest boundary

There is **no guest agent**. These remain explicit errors: guest entrypoints,
entrypoint/attached environments, Attached command execution, filesystem/config
sharing, private-key mounts, host themes, D-Bus/notification bridging, exact host
UID mapping and a required guest compositor security context. VM PID limits are
also rejected because the guest kernel owns its process table.

`Terminal` opens a host terminal and serial session. `KeepAlive` uses
`-no-shutdown` to retain QEMU after guest shutdown. **Background graphical
operation is unresolved**: closing the embedded GTK window terminates QEMU, and
there is no detachable frontend. The runtime refuses graphical Background.
Terminal plus Background is supported for a serial session: closing that
terminal preserves the VM. VM Attached command execution remains unavailable.

No interfaces means no managed NIC. Networked guests need pre-created TAPs and
[provisioned packet filtering](network-provisioning.md); there is no default
slirp NAT/pasta fallback. Configured DNS requires [authenticated proxy readiness](dns.md).

## Plans and raw flags

Planning does not start helpers, open audio devices or write guest state. It may
query explicit DNS for domain rules and prints placeholder broker endpoints.
It omits raw argument values because they can contain secrets; it is not a shell
script substituting for managed launch preparation.

`RunnerFlags` append argv to normal QEMU. `CreatorFlags` apply to installer
QEMU, including `zvr install --app APP`. Both warn: raw options can override
containment, disk and lifecycle controls, including adding networking to an
otherwise offline definition. [Hardware details](vm-hardware.md) cover display,
firmware, Windows-class guests and install-time state.

# zvr - VM runner

`zvr` runs QEMU from app YAML plus host-local VM-options JSON. Each instance has a
detached supervisor; networking requires owner-provisioned topology.

```sh
zvr pin /absolute/base.qcow2
zc new guest --vm --image /absolute/base.qcow2 --base-digest 'sha256:<64-hex>' \
    --memory 2048 --vcpus 2 --display None --loader-bios --cloud-init=false
zvr validate guest
zvr run guest --dry-run
zvr run guest
zvr status guest
zvr console guest
zvr stop guest
zvr reset guest --confirm
```

## Runtime options

`$XDG_CONFIG_HOME/zinc/runtime/vm/<app>.json` (default `~/.config/...`) holds
[version-1 disk/hardware options](../../docs/virtualization.md#app-intent-and-external-vm-options).
Replace the example pin with an independently authorized digest; launch never authorizes one.

- `--runtime-options /absolute/file.json` selects a file. `--base-digest`, `--disk`,
  `--display`, `--devices`, repeatable `--media`/`--forward` override it for one invocation.
- Media/forward overrides replace lists; `--clear-media`/`--clear-forwards` empty them.
  YAML paths require explicit options or a pin. Overrides never rewrite YAML/JSON.

Forwards: `[BIND:]HOST:GUEST[/TCP|UDP][@NIC]`; bracket IPv6 binds. Defaults: loopback,
TCP, first logical NIC. Provisioned mappings must match protocol, bind, both ports
and NIC exactly; wildcard publication cannot satisfy loopback intent.

## Network integration

Empty interfaces mean no managed NIC. Declared NICs need a matching
[manifest](../../common/adapters/network/README.md); `netns.ConfigureResolved` binds
runtime-only MACs before QEMU validation. Authored intent is preserved.

- `Service.Lookup` uses `common/adapters/network.Lookup`; CLI/supervisor inject
  `dnsproxy.Lookup`. Domain results are frozen snapshots with no host DNS fallback.
  Guest DNS needs matching provisioned proxy addresses/digest and an allowed route.
- `qemu.Layout.NetworkAttachments` maps InterfaceID to pre-created TapName, using
  `script=no,downscript=no`. Zinc creates no TAPs, namespaces, routes or publications;
  packet protocols, including SCTP, require TAPs rather than slirp.
- Raw `RunnerFlags` can add networking to an offline app, with warnings and no isolation
  guarantee. Provisioned topology checks actual QEMU NIC/backend arguments.

## Shared hardware and disk fields

- Positive RAM/whole CPU counts are required. `LoaderBIOS: false` selects UEFI;
  Secure Boot needs trusted firmware, TPM needs swtpm and compatible firmware.
- Empty Display selects None for Terminal, Compatible for DisableGpuAccess, otherwise
  Accelerated; GPU/Vulkan contradictions fail. See [hardware](../../docs/vm-hardware.md).
- CloudInit/Install provision first boot, with a public SSH key and explicit guest account;
  no implicit sudo/passwords. `ReadOnlyRootfs` needs guest support and forbids writable provisioning.
- `MinimizeFingerprint` uses neutral branding/generated MACs, preserving explicit MACs/UUIDs.

Overlays live under `$XDG_DATA_HOME/zinc/vms`; run verifies rather than recreates/resizes
them. Manifests bind disk/device/firmware identity. Reset deletes disk/NVRAM/TPM state,
keeping base images/options; see [disk rules](../../docs/virtualization.md#disks-and-firmware-state).

## Audio broker

PipeWire requires owner-deployed [WirePlumber policy](../../docs/audio.md).
A detached `audio.Prepare` holder reports private endpoints/socket over bounded pipes
before QEMU starts; QEMU receives those endpoints and `PIPEWIRE_REMOTE`, never a raw fallback.

Default/named grants are additive; Monitor is separate capture. ALSA needs real,
direction-correct kernel devices at launch; plans only convert paths to `hw:C,D`.
Codecs disable the ungranted input/output direction.

The supervisor owns the holder lifeline. Startup failure, exit or stop closes it;
policy/heartbeat/holder loss revokes audio and appears in status/logs without killing
the guest or discarding storage. Launch never modifies host audio services.

## Supervision and lifecycle

The supervisor confirms guest/QMP readiness before launcher acknowledgement.
Restart revalidates intent, pin, manifest and host support; this is process supervision,
not guest/application health. See [lifecycle](../../docs/virtualization.md#supervision).

- Preparation: fixed 10 minutes (`app.DefaultPreparationTimeout`) across DNS, pin,
  dependencies, disks, TPM, audio and QEMU. `Service.PreparationTimeout`: zero defaults,
  negative fails. IPC/ack bounds are separately 30 seconds, plus cleanup grace.
  Status bytes never extend the deadline; timeout rolls back unacknowledged startup.
- Autorestart: nonzero unexpected exits only, 1/2/4/8/16-second backoffs, five retries;
  five stable minutes reset the burst. No restart after clean exit, stop, preparation
  failure or uncertain cleanup. Persisted stop intent cancels pending restart; live reset fails.
- Terminal opens a serial session; KeepAlive uses `-no-shutdown`. Serial Background
  survives terminal closure; graphical Background is refused (GTK closure kills QEMU).
  Guest commands/env, filesystem/bus integration and exact UID/GID provisioning are errors.

## Planning, raw flags and installation

- Plans start no helpers, open no devices and write no state; explicit DNS may be queried.
  Audio endpoints are placeholders; raw values are omitted as potential secrets.
  A plan cannot replace managed launch preparation.
- `RunnerFlags` append literal argv to QEMU; `CreatorFlags` apply only to installer QEMU
  (`zvr install --app guest`). Both warn: they can override containment, disks and lifecycle.
- Installation needs `--resume` to write an existing target and prints the completed pin.
  The disk hash excludes NVRAM; Secure Boot refuses untrusted adoption and TPM-sealed
  state is not cloned. A disk alone is not a [machine backup](../../docs/vm-hardware.md).

## Checks

`make check` uses pinned Podman tooling; canonical-common migration work needs a
temporary workspace instead of stale vendors. [E2E](../e2e/README.md) covers offline
QEMU lifecycle and separately gated provisioned networking.

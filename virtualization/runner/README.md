# zvr - VM runner

`zvr` combines authored app YAML, separate host-local VM options, and explicitly
provisioned network topology. A detached supervisor owns each running instance.

```sh
zvr pin /absolute/base.qcow2
zc new guest --vm --image /absolute/base.qcow2 --base-digest sha256:... \
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

`$XDG_CONFIG_HOME/zinc/runtime/vm/<app>.json` (default `~/.config/...`) stores
Version 1, AppNameID, Image, BaseDigest, DiskSizeGiB, Display, Devices,
InstallMedia and ForwardPorts. Image/AppNameID bind the options to app intent.
The disk pin is required; launch never authorizes a newly calculated digest.

`--runtime-options /absolute/file.json` selects an explicit file. `--base-digest`,
`--disk`, `--display`, `--devices`, repeatable `--media` and `--forward` override
it for one invocation. Media/forward overrides replace lists; `--clear-media`
and `--clear-forwards` empty them. Running YAML by path requires explicit options
or an explicit pin. CLI overrides never rewrite app YAML or options files.

Forward syntax: `[BIND:]HOST:GUEST[/TCP|UDP][@NIC]`, with brackets for IPv6 binds.
Defaults are loopback, TCP and the first declared logical NIC. Mappings must
already be provisioned and match protocol, bind address, both ports and NIC
exactly. A wildcard publication cannot satisfy a loopback request.

## Network integration

Empty Interfaces gives no managed NIC. Declared NICs require an authoritative
[network manifest](../../common/adapters/network/README.md). The runner loads it,
checks all publications, and applies `netns.ConfigureResolved` before QEMU
validation. Assigned MACs only modify a runtime copy; authored intent remains
unchanged for validation, auditing and subsequent restarts.

`Service.Lookup` has the `common/adapters/network.Lookup` signature. The CLI and
supervisor wire `dnsproxy.Lookup`, which uses each policy owner's explicit DNS
configuration. Domain results are frozen launch snapshots; no host resolver
fallback is allowed. Configured guest DNS still needs the provisioner's matching
proxy addresses/digest and a permitted route to that proxy.

`qemu.Layout.NetworkAttachments` maps InterfaceID to a pre-created TapName.
QEMU uses `script=no,downscript=no`. The adapter does not create namespaces,
TAPs, routes, listeners or host firewall rules. SCTP and other packet protocols
use provisioned TAPs, never a claimed slirp approximation.

With no declared NICs, explicit raw RunnerFlags may add networking. This is a
warned opt-out: such a run is not guaranteed network-isolated. With a provisioned
topology, the network adapter checks the actual QEMU NIC/backend arguments.

## Shared hardware and disk fields

- ResourcesMeta supplies positive RAM and a positive whole VM CPU count.
- LoaderBIOS false selects UEFI. SecureBoot requires matching trusted firmware;
  TPM requires swtpm and compatible firmware.
- Empty runtime Display infers None for Terminal, Compatible for DisableGpuAccess,
  otherwise Accelerated. Contradictions with GPU denial/Vulkan are rejected.
- ImageMeta.CloudInit explicitly enables provisioning. PublicSSHKeyPath contains
  a public key; InternalUserMeta selects a guest account without implicit sudo or
  passwords. Install is first-boot provisioning, not a command replayed per launch.
- ReadOnlyRootfs attaches the root block device read-only, not in snapshot mode.
  Installation and writable cloud-init provisioning cannot use it. The base OS
  must itself support a read-only root.
- MinimizeFingerprint uses neutral SMBIOS branding and locally administered
  generated MACs. Explicit/provisioned MACs and stable UUIDs are preserved.

Overlays remain under `$XDG_DATA_HOME/zinc/vms`. Run never recreates or resizes an
existing overlay; it verifies the backing image, format and size. Instance
manifests prevent silent base-pin/device/firmware changes. Reset explicitly
deletes guest disk/NVRAM/TPM state while preserving base images and runtime options.

## Audio broker

Every PipeWire grant uses `common/adapters/audio.Prepare` in a detached holder.
The complete directional request travels over bounded inherited pipes. The
holder reports readiness, private endpoint names and its private socket before
QEMU starts. QEMU receives only these endpoints and `PIPEWIRE_REMOTE` for that
socket; naked named-host routing is never treated as isolation.

Default and named PipeWire selections are additive. Monitor is a separate capture
endpoint. ALSA PCM/control selections use the shared plan and must be real kernel
ALSA character devices at launch. Planning only converts their paths to hw:C,D.
Microphone/monitor codecs disable host output voices; playback disables host input.

The supervisor owns the holder lifeline. Startup failure, guest exit and explicit
stop close it; holder shutdown calls Session.Close. Lost policy, heartbeat or
holder process revokes audio and is reported in status/logs. Audio failure does
not force-kill the guest or discard its storage. The shared WirePlumber policy
must be installed by the host owner; this runner never modifies host audio services.

## Supervision and lifecycle

The detached supervisor receives the complete launch snapshot over private pipes,
confirms guest/QMP readiness, then waits for launcher acknowledgement. Restarting
revalidates the authored snapshot, disk pin, manifest and current host capabilities.

Initial supervisor readiness has a fixed 10-minute preparation budget
(`app.DefaultPreparationTimeout`) covering DNS lookups, base verification,
dependencies, disks, TPM, audio and QEMU startup. Internal callers/tests can set
`Service.PreparationTimeout`; zero uses the default and negative values fail.
This is separate from 30-second IPC message/acknowledgement bounds and the cleanup
grace period. Timeout closes the launcher lifeline and rolls back unacknowledged
startup; incoming status bytes never extend the preparation deadline.

Autorestart handles unexpected nonzero QEMU exits with 1, 2, 4, 8 and 16 second
backoffs, at most five retries per burst. Five minutes of stable execution resets
the burst. Clean QEMU exit, explicit stop, preparation failure and uncertain
cleanup do not restart. This is process supervision, not guest OS/application health.
Generation-specific stop intent is persisted before waiting for a startup lock,
so manual stop also cancels pending restart. Reset refuses a live supervisor.

Terminal opens the configured host terminal and serial session. KeepAlive uses
`-no-shutdown` to retain QEMU after guest shutdown. Background graphical windows
are refused: embedded GTK closure terminates QEMU, and a detachable frontend is
not implemented. Serial Terminal plus Background retains the guest after closing
the terminal. Guest attached commands, entrypoints/environments, filesystem/bus
integration and exact UID/GID provisioning remain explicit errors.

## Planning, raw flags and installation

Plan performs no audio connection, helper startup, device opening or state writes.
It prints placeholder private audio endpoints and omits raw argument values,
which can contain secrets. Domain rules may query explicitly configured DNS.
The printed managed plan is not a shell replacement for launch-time preparation.

RunnerFlags append argv directly to normal QEMU; CreatorFlags append only to
installer QEMU (`zvr install --app guest` can supply hardware defaults/flags).
Each value is one argument, never shell-evaluated. Both warn before execution:
raw arguments can override containment, disks and lifecycle controls.

Installation requires --resume to write an existing target. The completed disk
pin is printed for runtime options. Installed NVRAM is not covered by the disk
hash; Secure Boot refuses untrusted adoption. TPM-sealed state is not cloned
automatically. A disk alone is not a complete machine backup.

## Checks

`make check` uses pinned Podman tooling. During migration use a temporary workspace
against canonical common code instead of stale vendors. `../e2e` contains a real
QEMU offline lifecycle smoke test and a separately gated provisioned-network suite.

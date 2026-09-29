# Zinc audio policy, protocol 1

Linux baseline: PipeWire 1.4.11 / WirePlumber 0.5.14. Recheck new versions with `make isolated`.
[Protocol/trust boundary](PROTOCOL.md).

## Installation is a separate operator action

Runners never install/restart services. Deploy after explicit target-desktop approval:

| Repository source | Per-user destination |
| --- | --- |
| `90-zinc-audio.conf` | `$XDG_CONFIG_HOME/wireplumber/wireplumber.conf.d/90-zinc-audio.conf` |
| `scripts/zinc-audio.lua` | `$XDG_DATA_HOME/wireplumber/scripts/zinc-audio.lua` |
| `scripts/lib/zinc-audio-*.lua` | `$XDG_DATA_HOME/wireplumber/scripts/lib/` |

- XDG defaults: `~/.config`, `~/.local/share`; files: session-user readable, sandbox-unwritable.
  Preserve configuration/check conflicts. Load via approved WirePlumber restart/new login, never during app launch.
- Requires `main`/`policy` profile, native manager socket, PipeWire loopback/adapter modules.
  Custom profiles must require `policy.zinc-audio` and its [dependencies](90-zinc-audio.conf).
  Conflicting rules/other managers unsupported. No `pw-cli`/`pw-loopback`, cgo, new Go deps or device permission changes.
- Rollback: stop broker-using apps, remove only deployed Zinc files, reload after approval.
  New launches fail missing-policy; never substitute a raw host audio socket.

## Shared Go API

Import `github.com/crispuscrew/zinc/common/adapters/audio`.

```go
session, err := audio.Prepare(ctx, audio.Request{
    RuntimeDir: runtimeDir,
    AppID: appName,
    InstanceID: instanceName,
    Audio: cfg.AudioMeta,
})
```

Keep `ctx`/`session` through app exit. `Prepare` prepares every PipeWire grant before returning; `session.Close()` is idempotent.
Watch `session.Done()`/report `session.Err()`: failure revokes audio without terminating the app. Never reopen the host socket.

- `session.Socket`: native PipeWire, 0600; mount only this socket. Runtime directory: absolute/private/broker-owned.
  Rootless UID mapping must allow app connections without exposing that directory.
- `session.Endpoints`: independent `{Direction, Name, Target}`; private `node.name` is `Name`, host reporting is `Target`.
- `session.ALSA`: direction, canonical path, `hw:card,device`, optional explicit control path.
  Caller verifies character devices/permissions and handles mounts/backend opening.
- `session.Environment()`: merge QEMU remote override; clear inherited `PIPEWIRE_NODE`/`PIPEWIRE_PROPS`/config overrides;
  preserve display `XDG_RUNTIME_DIR`.
- QEMU: Playback -> `out.name=Name`; Microphone/Monitor -> `in.name=Name`, separate capture audiodevs.
  Disable unwanted backend direction; HDA microphone codecs also have output pins. ALSA -> `in.dev`/`out.dev`,
  double keyval commas; no host ALSA devices in guests. [Mapping](../../virtualization/runner/domain/qemu/audio.go).
- ALSA-only needs no PipeWire. `Monitor.ALSADevices` fails: PCM paths do not establish a reviewed loopback route.
- Container holder: full request on inherited FD 4, readiness on FD 3; old microphone boolean alone cannot authorize.
  App disappearance, policy failure or SIGTERM/SIGINT exits/revokes the holder.

## Semantics

- Exact `node.name` selectors, additive fields/lists, coalesced direction/node duplicates.
  Prepare snapshots defaults (Monitor: default sink monitor). Missing/ambiguous names fail; loss revokes without retargeting.
- Only private virtual endpoints are visible. Playback's own monitor contains at most this instance's audio;
  a Monitor grant separately captures the selected host sink mix. Trusted host apps can record/reroute audio.
- PulseAudio-only apps need a separately restricted frontend, not provided here. Never expose the host Pulse socket.

## Checks

From here: `make test`, `make lint`, `make isolated`. [Makefile](Makefile): Podman/digest-pinned Go, no network,
read-only sources/existing vendor trees. `isolated` requires installed `pipewire`, `wireplumber`, `dbus-run-session`,
stock WirePlumber config/linking/access scripts, `aplay`, `arecord`, ALSA PipeWire plugin.
Make sets `ZINC_AUDIO_TEST_POLICY`/`TMPDIR`; binary: `/tmp/opencode/zinc-audio-check/audio.test` (`OUTPUT` override).
Private daemons, hardware discovery disabled; no real microphone/speaker, user socket or host service.
`make test` skips isolated cases without policy environment; no live-daemon pass implied.

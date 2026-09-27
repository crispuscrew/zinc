# Zinc audio policy, protocol 1

Compatibility baseline: PipeWire 1.4.11 and WirePlumber 0.5.14 on Linux.
The isolated integration suite runs the actual installed daemons, their stock
linking/access scripts, and this policy. New versions require the same suite.

## Installation is a separate operator action

Nothing in the Go runner installs files or restarts a host service. Deploy only
after explicit approval for the target desktop. Ship these exact files:

| Repository source | Per-user destination |
| --- | --- |
| `90-zinc-audio.conf` | `$XDG_CONFIG_HOME/wireplumber/wireplumber.conf.d/90-zinc-audio.conf` |
| `scripts/zinc-audio.lua` | `$XDG_DATA_HOME/wireplumber/scripts/zinc-audio.lua` |
| `scripts/lib/zinc-audio-*.lua` | `$XDG_DATA_HOME/wireplumber/scripts/lib/` |

The standard XDG defaults are `~/.config` and `~/.local/share`. Install the files
readable by the session user, not writable by sandbox applications. Preserve
existing configuration and check for a conflicting file before replacement.
A newly approved WirePlumber restart or new login is required to load the policy.
Do not restart a live audio session as part of an application launch.

The policy requires the ordinary `main` or `policy` profile, the native PipeWire
manager socket, and PipeWire's loopback and adapter modules. No pw-cli/pw-loopback
executable, cgo linkage, new Go dependency, or host device permission change is
required. Custom profiles must explicitly require `policy.zinc-audio` and its
dependencies. Another session manager or a conflicting access rule is unsupported.

Rollback: stop applications using this broker, remove only the deployed Zinc
files, and reload the session after approval. New launches then fail with a
missing-policy error. Never replace that failure with a raw audio socket.

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

`ctx` and `session` must outlive the application process. `Prepare` resolves and
prepares every PipeWire grant before returning. `session.Close()` is idempotent.
Watch `session.Done()` and report `session.Err()`; failures revoke the audio
socket and do not terminate the guest/container. Never reopen the host socket.

- `session.Socket`: restricted native PipeWire socket, mode 0600. Mount only this
  socket into a container. Rootless UID mapping must permit its intended app user
  to connect; do not make the host runtime directory accessible as a workaround.
  The supplied runtime directory must be private and owned by the broker user.
- `session.Endpoints`: independent `{Direction, Name, Target}` entries. `Name` is
  the private application-facing node.name. `Target` is host-side reporting data.
- `session.ALSA`: independent PCM records with direction, canonical path,
  `hw:card,device`, and optional explicitly granted control path. The caller owns
  character-device verification, permissions, device mounts, and backend opening.
- `session.Environment()`: PipeWire remote override for QEMU. Merge it while
  clearing inherited PIPEWIRE_NODE/PIPEWIRE_PROPS and config overrides. Preserve
  the display's XDG_RUNTIME_DIR.

For QEMU, map Playback to `out.name=Name`; Microphone and Monitor to `in.name=Name`.
Use separate audiodevs for separate capture endpoints; do not mix microphone and
monitor implicitly. Disable the unwanted backend direction. HDA microphone
codecs also have output pins: the codec name alone is not direction enforcement.
ALSA records map to QEMU's `in.dev`/`out.dev` and require doubled commas in its
keyval syntax. The guest never receives a host ALSA character device.

An ALSA-only request needs no PipeWire daemon. `Monitor.ALSADevices` is rejected:
a PCM path does not identify a reviewed hardware/software loopback route.

The container adapter passes the complete request on inherited FD 4 to its
existing hidden holder. FD 3 reports readiness. The old microphone CLI boolean
cannot authorize audio by itself. The holder exits/revokes when the app disappears,
the policy fails, or it receives SIGTERM/SIGINT.

## Semantics

Selectors are exact node.name strings. Fields and lists are additive. Duplicate
selections resolving to the same direction/node are coalesced. Session defaults
are snapshotted at prepare time; Monitor's default is the default sink monitor.
Unavailable or ambiguous named nodes fail preparation. Target loss revokes the
session; there is no fallback or automatic retarget to a different device.

Only private virtual endpoints are visible. A private playback sink's own monitor
can at most contain this instance's audio; it does not expose the host sink mix.
The named monitor grant is a separate source fed by the selected host sink monitor.
Trusted host applications can still record or reroute host audio; this boundary
isolates sandbox clients, not the desktop owner from their own session.

This adapter serves native PipeWire. PulseAudio-only applications need a separately
restricted Pulse frontend; this implementation does not expose the host Pulse
socket or pretend that it implements that frontend.

## Checks

Run `make test`, `make lint`, and `make isolated` from this directory. Compilation
uses the repository's digest-pinned Go container, no network, canonical source,
and existing YAML code. Test-only module overlays avoid changing vendor trees.

`make isolated` starts fresh daemons in private temporary directories with all
hardware discovery disabled. Synthetic PCM tests additionally use existing aplay,
arecord, and the ALSA PipeWire plugin. No real microphone, speaker, user PipeWire
socket, or host service is used. The generated test executable is under
`/tmp/opencode/zinc-audio-check` by default.

The consuming runners must integrate the resolved endpoint mapping. In particular,
VM-side validation must permit brokered Monitor and additive selections, and
shared authoring warnings must stop claiming host-monitor denial is impossible.

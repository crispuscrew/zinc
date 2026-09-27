# Directional audio grants

Each `AudioMeta` direction is an `AudioDevice` mapping:

```yaml
AudioMeta:
  Playback:
    PipeWireDefault: true
    PipeWireDevices: [alsa_output.example.analog-stereo]
  Microphone:
    ALSADevices: [/dev/snd/controlC0, /dev/snd/pcmC0D0c]
  Monitor: {}
```

Names and device paths above are host-specific placeholders. Omitted directions
or `{}` grant nothing. `PipeWireDefault`, `PipeWireDevices` and `ALSADevices`
are additive within a direction, not alternate modes. Selectors are exact
PipeWire `node.name` strings; ambiguous or unavailable nodes fail preparation.
Selections resolving to the same direction/node are coalesced.

`Playback` writes sound, `Microphone` captures an input, and `Monitor` records a
selected sink's mix. Monitor is a separate capture grant, supported through the
broker for containers and VMs. `Monitor.ALSADevices` is rejected because a PCM
path alone does not identify a reviewed loopback route.

## Opt-in WirePlumber deployment is required

Brokered PipeWire needs the Zinc protocol-1 WirePlumber policy. The compatibility
baseline is PipeWire 1.4.11 and WirePlumber 0.5.14; different versions need the
isolated integration suite. Ordinary native session-manager defaults alone are
insufficient.

The host owner must explicitly deploy the config, script and library files
listed in [integration/wireplumber](../integration/wireplumber/README.md), and
load them through a separately approved session reload/new login. App launch
installs nothing and never restarts a host audio service. Custom profiles need
the policy component and dependencies; conflicting access rules or another
session manager are unsupported.

Missing/incompatible policy fails preparation. There is no fallback to the raw
host PipeWire or Pulse socket. Rollback removes only the deployed Zinc files
after stopping brokered apps; new PipeWire launches then fail closed.

## Broker boundary and lifetime

`common/adapters/audio.Prepare` resolves the full request before returning a
private mode-0600 native PipeWire socket and private endpoint names. Defaults
are snapshotted at preparation time; Monitor's default is the default sink's
monitor. Target loss revokes the session instead of selecting another device.

Only private virtual endpoints are exposed. The monitor of a private playback
sink can contain that instance's own sound, not the host sink mix. Access to
the host mix requires the separate Monitor grant. Trusted host programs can
still record or reroute host audio: this is a sandbox-client boundary, not a
restriction on the desktop owner.

The broker session/context must outlive the app. Holders report readiness over
private inherited pipes and keep the session alive after the launcher returns.
Cancellation, app exit, policy loss or holder failure revokes the socket.
Failures are reported; audio loss does not force-kill a VM or discard storage.

Containers mount only the restricted socket. Their user mapping must allow the
intended user to connect; exposing the whole runtime directory is not a fix.
QEMU receives private endpoints and a `PIPEWIRE_REMOTE` override. Playback
maps to `out.name`; microphone and monitor map to separate `in.name` endpoints.
Unwanted backend directions are disabled: an HDA codec's name alone does not
guarantee directional isolation.

## ALSA and client compatibility

ALSA grants name exact playback/capture PCM nodes and optional explicitly
granted control nodes. Launch checks actual kernel character devices and their
direction, permissions and paths; it does not expose all of `/dev/snd`.
An ALSA-only request needs no PipeWire daemon. Control-node access is broader
than opening a single PCM stream and should be reviewed accordingly.

For containers these become device grants. For VMs QEMU opens the selected
`hw:card,device` backend; the guest receives emulated audio hardware, never a
host character-device mount. Multiple captures remain separate.

The broker serves native PipeWire. PulseAudio-only clients need a separately
restricted Pulse frontend, which this implementation does not supply. Selecting
playback in YAML cannot make an incompatible application speak native PipeWire.

The isolated WirePlumber suite uses fresh private daemons and synthetic audio
with hardware discovery disabled. See its README for `make test`, `make lint`
and `make isolated`; running a unit test alone is not a deployment compatibility
check for the installed desktop policy.

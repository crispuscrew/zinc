# Directional audio grants

Each `AudioMeta` direction is an additive `AudioDevice` mapping:

```yaml
AudioMeta:
  Playback:
    PipeWireDefault: true
    PipeWireDevices: [alsa_output.example.analog-stereo]
  Microphone:
    ALSADevices: [/dev/snd/controlC0, /dev/snd/pcmC0D0c]
  Monitor: {}
```

Replace host-specific names/paths. Omission/`{}` grants nothing; default, named
PipeWire and ALSA selections add together. Exact `node.name` selectors must resolve
unambiguously; unavailable nodes fail. Duplicate direction/node selections coalesce.
Playback writes; Microphone captures input; Monitor separately captures a sink mix
in both runtimes. `Monitor.ALSADevices` errors: PCM alone identifies no reviewed loopback.

## Opt-in WirePlumber deployment is required

Follow [protocol-1 deployment and rollback](../integration/wireplumber/README.md#installation-is-a-separate-operator-action):
owner-deployed config/scripts/libraries, separately approved reload/new login.
App launch installs/restarts nothing. Stock defaults alone are insufficient;
custom profiles need policy dependencies; conflicting rules/session managers are unsupported.
Missing/incompatible policy fails closed, never exposing raw PipeWire/Pulse sockets.
Rollback stops brokered apps and removes only deployed Zinc files; new launches then fail.

Baseline: PipeWire 1.4.11, WirePlumber 0.5.14. Other versions require the
[isolated suite](../integration/wireplumber/README.md#checks): `make test`, `make lint`,
`make isolated`; private daemons, synthetic audio, hardware discovery disabled.
Unit tests alone do not establish desktop-policy compatibility.

## Broker boundary and lifetime

[`audio.Prepare` and runtime integration](../integration/wireplumber/README.md#shared-go-api)
resolve the full request before returning private endpoints/socket (0600).
Defaults snapshot at preparation; Monitor defaults to the default sink monitor.
Target loss revokes, never retargets. A private playback monitor contains only
instance sound; host mix needs Monitor. Trusted host programs can still record/reroute.

Session/context must outlive the app. Holders signal readiness through inherited
private pipes and survive launcher return. Cancellation, app exit, policy/holder loss
revokes the socket and reports failure without force-killing a VM or discarding storage.
Mount only the restricted socket with usable app UID mapping, never the whole runtime dir.
QEMU gets `PIPEWIRE_REMOTE` and private `out.name`/separate `in.name` endpoints;
disable unwanted directions. HDA codec names alone do not isolate directions.

## ALSA and client compatibility

ALSA grants exact playback/capture PCM and optional control nodes, never all `/dev/snd`.
Launch checks kernel character devices, direction, permissions and paths. Controls
grant broader access than one PCM stream; ALSA-only needs no PipeWire daemon.
Containers get devices; QEMU opens `hw:card,device` and guests get emulated hardware,
never host device mounts. Captures stay separate.
PulseAudio-only clients need a separately restricted frontend, not supplied here;
a YAML grant cannot make them native PipeWire clients.

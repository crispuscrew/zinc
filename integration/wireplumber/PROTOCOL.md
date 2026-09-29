# Broker protocol and security boundary

## Controller

1. Broker connects to `pipewire-0-manager`, sets `zinc.audio.protocol=1` and JSON `zinc.audio.request`:
   `app`, `instance` (`<instance>:<token>`), fresh 32-hex-digit `token`, `selections`.
   Require server-stamped `pipewire.sec.socket=pipewire-0-manager`, absent `pipewire.sec.engine`.
   `application.name` is not a credential; host manager clients are trusted.
2. Resolve exact selectors; load one trusted local loopback module per direction/node. Private endpoints
   have generic descriptions and unguessable instance names; separate host bridge streams are never granted.
   Playback: virtual sink -> host sink; Microphone: host source -> virtual source;
   Monitor: host sink monitor ports -> distinct virtual source. Bridges target `object.serial`, with
   `dont-fallback`, `dont-reconnect`, `dont-move`; passive streams avoid intentional capture without consumers.
3. All private endpoints, bridges and host links must exist, then pass a PipeWire sync barrier before ready.
   Replies use controller client properties, not shared metadata: `zinc.audio.version=1`,
   `zinc.audio.status` (`ready`/`error`/`revoked`), `zinc.audio.error`, JSON `zinc.audio.endpoints`.
   Ready attests graph setup, not hardware health/latency. Go checks version, endpoint names (`za.<token>.*`),
   uniqueness, direction/target and every selection's coverage before creating a listener.

## Application socket

Only after ready does Go create/synchronize a security-context listener carrying:

- `pipewire.sec.engine=com.github.crispuscrew.zinc`
- `pipewire.sec.app-id=<app>`
- `pipewire.sec.instance-id=<instance>:<random token>`
- `pipewire.access=zinc-audio-v1`

- PipeWire 1.4.11 filters client changes to `pipewire.access`/`pipewire.sec.*`. `module-access` grants
  nothing for the pre-stamped category: clients start `PW_ID_ANY=0`, blocked until core access is granted.
  WirePlumber 0.5.14 `access-default.lua` does not grant this category; the supplied rule enforces zero fallback.
  No grant-then-revoke race/permissive `PW_ID_ANY`.
- Each connection/reconnect is checked independently. Grants: core RX (octal 0500), own client/stream objects,
  client-node/link factories, instance-private endpoints/ports. Denied: physical nodes, bridges, other instances,
  hardware factories, global metadata and other graph capabilities.
- Before ordinary target selection, the linking hook accepts authorized private endpoints/directional defaults only;
  missing/unauthorized targets stop fallback. Object permissions also deny unauthorized direct links.
- Only trusted WirePlumber-owned private endpoints qualify. App-controlled properties cannot identify a
  controller or elevate a sandbox into another instance.

## Lifetime

- Every second Go sends `zinc.audio.ping`; after graph-health checks policy echoes `zinc.audio.pong`.
  Only the expected value with matching version refreshes the 4s timeout. Expiry revokes; policy error revokes immediately.
- Closing the revocation pipe destroys the listener and, on tested PipeWire, accepted clients. Controller removal
  clears prior explicit grants/destroys bridges; policy exit destroys local modules. Listener shutdown alone is insufficient.
- Target ID, `object.serial`, name, class and trusted origin must stay unchanged; loss/change revokes, never retargets.
  No status, restart, failure or unsupported-server path returns the host socket.

## Limits

- Trust: host session manager, manager clients, PipeWire. Deploy config/scripts together; conflicting permission
  policy or another host audio socket breaks isolation. Runner mount/UID policy must prevent bypass paths/devices.
- [Native Go client](../../common/adapters/audio/connection.go): control objects only; upstream loopback transports samples.
  Bounds: 1 MiB frame bodies, POD lengths, 4096 dictionary entries; reject duplicate keys/negative or oversized counts,
  preserve partial frames, handle Ping/Pong/short writes, close unexpected received FDs.
- Preparation defaults to 10s (`Request.Timeout`, capped by context deadline); connect/writes have separate 5s bounds.
  Policy accepts 1..32 selections, node names <=1024 bytes; generated Unix socket path <=107 bytes.

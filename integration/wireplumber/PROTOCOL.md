# Broker protocol and security boundary

## Controller

A broker connects to `pipewire-0-manager` and sets `zinc.audio.protocol=1` and a
JSON `zinc.audio.request` containing app, fresh instance token, and selections.
The policy authenticates the connection using the server-stamped
`pipewire.sec.socket` and absence of a sandbox engine. Application.name is not an
authentication credential. Host processes with manager access are trusted.

The policy resolves exact selectors and checks the originating client is not a
sandbox. It loads one trusted local loopback module per resolved direction/node.
Application-facing nodes have generic descriptions and unguessable per-instance
names. Host-side stream nodes are separate and never granted to sandbox clients.

Playback is a virtual sink feeding a named host sink. Microphone is a named host
source feeding a virtual source. Monitor captures the named host sink's monitor
ports into a distinct virtual source. Trusted bridge streams use object.serial
targets, dont-fallback, dont-reconnect, and dont-move. They are passive so they do
not intentionally keep hardware capture active without a downstream consumer.

Ready is sent only after all private endpoints, their trusted bridge streams, and
their host-side links exist, followed by a PipeWire synchronization barrier. It
does not assert physical hardware health or a measured latency guarantee. Errors
are sent on the controller's own client properties, not shared mutable metadata.

## Application socket

Only after ready does Go create a security-context listener. Its properties carry:

- `pipewire.sec.engine=com.github.crispuscrew.zinc`
- `pipewire.sec.app-id=<app>`
- `pipewire.sec.instance-id=<instance>:<random token>`
- `pipewire.access=zinc-audio-v1`

PipeWire 1.4.11 filters client updates to pipewire.access and all pipewire.sec.*
properties. module-access sees the pre-stamped access category and supplies no
permissions. New clients start with PW_ID_ANY=0 and cannot progress until policy
grants core access. WirePlumber 0.5.14 access-default.lua does not grant the custom
category; the supplied access rule also enforces a zero fallback for Zinc clients.
There is no grant-then-revoke race and no permissive PW_ID_ANY entry.

For every connection the policy grants only core RX, its own client/stream objects,
client-node/link factories, and the instance's private endpoints/ports. Physical
nodes, bridge streams, other instances, hardware factories, global metadata, and
other graph capabilities stay denied. RX is octal 0500 in PipeWire's ABI.

The linking hook runs before ordinary target selection. A sandbox stream can
select an authorized private endpoint or its direction's private default. A
missing/unauthorized target stops event processing before fallback hooks. Daemon
object permissions also enforce denial against direct native link requests.

The controller is not identified through application-controlled node properties.
Private endpoints are accepted only when owned by WirePlumber's trusted process.
User-written properties cannot elevate a sandbox into a controller or another
instance. Multiple connections and reconnects are all evaluated independently.

## Lifetime

The Go controller sends a heartbeat once per second. Policy acknowledges on its
client properties after processing graph health. No acknowledgment for four
seconds revokes the application socket. Explicit policy error revokes immediately.
Closing the revocation pipe destroys the listener (and on the tested PipeWire
version its accepted clients). Controller removal tells policy to clear previous
explicit grants and destroy bridge nodes. Policy process exit destroys its local
modules. Go does not rely on listener shutdown alone to remove graph routing.

Selected node identity is checked by ID, object.serial, name, class, and trusted
origin. Removal or change revokes instead of resolving the old ID to a new device.
No status, restart, failure, or unsupported-server path returns the host socket.

## Limits

The host session manager, host manager clients, and PipeWire itself are trusted.
Installing conflicting third-party permission policy or handing a sandbox another
host audio socket invalidates the deployment assumptions. Deploy configuration
and scripts together. The runner's broader mount/UID policy remains responsible
for preventing paths or additional devices that bypass this broker.

The native Go client binds only control objects. It bounds message/POD lengths,
rejects duplicate dictionary keys and negative/oversized counts, preserves partial
frames, handles Ping/Pong and short writes, and closes unexpected received FDs.
It does not implement PipeWire sample transport; upstream loopback modules do.

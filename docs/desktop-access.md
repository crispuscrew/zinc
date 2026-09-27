# Display, GPU and session bus

## Wayland identity

For a graphical container, Zinc normally binds a private socket and registers
it through `wp_security_context_v1`. The app mounts that socket instead of the
compositor's raw socket. Identity is supplied before the app connects:

- `sandbox_engine`: `com.github.crispuscrew.zinc`.
- `app_id`: stable app identity across restarts/instances.
- `instance_id`: runtime name, also reported by `zcr where`.

The listener must already be bound when registered. The compositor continues
accepting after the registration connection closes; a hidden holder retains the
revocation descriptor until the app exits, then closes it and removes the socket.
Launch waits for holder readiness before asking Podman to mount anything.

If the compositor lacks the protocol, the default is a raw-socket fallback with
a warning. `DisplayMeta.RequireSecurityContext: true` refuses that downgrade;
`DisableSecurityContext: true` explicitly selects passthrough. Setting both is
invalid. Other setup errors fail launch rather than pretending a context exists.
The `zinc.wayland` label records the result, not merely the requested setting.

Zinc supplies authenticated context identity; **the compositor chooses policy**
for that identity. Protocol tagging is not a promise that every compositor blocks
every privileged operation. The container kernel boundary remains separate.
Guests do not have a guest-compositor security-context bridge; VM
`RequireSecurityContext` is rejected.

## GPU and dimensions

GPU access is opt-out: set `DisplayMeta.DisableGpuAccess: true` to deny it.
Exposing `/dev/dri` increases the host driver attack surface and permits GPU
resource exhaustion. There is no enforced VRAM cap in `ResourcesMeta`.

The kernel dmem controller needs driver-registered regions and usable limits;
Zinc does not advertise a memory limit merely because the controller exists.
QEMU's Venus `hostmem` is an address-space window, not a VRAM quota, and a
compatible display's framebuffer size is derived from resolution, not a GPU cap.
Display dimensions and Vulkan also have runtime-specific validation; see
[VM hardware](vm-hardware.md).

## Filtered D-Bus

Empty `DBusMeta` grants no session bus. `Talk` names services the app may call;
`Own` names it may claim. Trailing `.*` policies match subnames and can grant
future matching services, so review their breadth. `KeepUserID` is required so
the intended app user can connect to the broker's socket.

`xdg-dbus-proxy` runs in a separate helper container, outside the application
pod/process boundary. Only the proxy holds the real bus socket; the app gets
the filtered per-instance socket. A requested bus without a resolvable host bus
fails launch. No VM guest-side bus bridge is implemented.

The helper image is built with `make -C container/runner netfilter-image` and
selected by `ZINC_NETFILTER_IMAGE`. It also applies container nft policy inside
already-provisioned namespaces; it does not provision host topology.

## Attribution

`zcr bus --json` maps running proxy host PIDs to `app@instance`. `zcr where APP
--json` reports runtime paths, proxy and socket; no bus grant reports no bus.
A desktop can query `org.freedesktop.DBus.GetConnectionUnixProcessID` and join
that kernel-derived PID to the runner's table.

The app is not asked to claim a special attribution name: proxy `--own` grants
permission for the app to claim one, not an authenticated host identity. The
mapping comes from resources Zinc created. A proxy opens one upstream connection
per client, so zero live clients means no host connection and several clients
can share one proxy PID. PID attribution is best effort across time because
process IDs can be recycled; query live state rather than cache indefinitely.

Runtime dot-separated names can be ambiguous with dotted app IDs; the store's
whole-app-name match wins where both readings exist. Public commands accept the
unambiguous `app@instance` notation.

## Notifications

Nonzero `NotificationMeta` requires a bus grant reaching
`org.freedesktop.Notifications`. The notification filter sits between app and
D-Bus proxy because name filtering alone cannot constrain message bodies.

- `Disabled` refuses Notify; `Silenced` accepts and drops it.
- `UseCustomPrefix` applies `CustomPrefix` to summaries.
- `AllowedActions`, `AllowedLinks`, `AllowedProlonged` control action buttons,
  anchor markup and timeouts longer than the ten-second clamp.

A zero notification mapping installs no filter; it is not equivalent to an
enabled filter with explicit denials. Non-Notify traffic, including file
descriptors used by portals, is relayed without re-encoding. Notify is decoded
and encoded because changing a string shifts D-Bus alignment. VM notifications
are rejected because there is no guest-side bus bridge.

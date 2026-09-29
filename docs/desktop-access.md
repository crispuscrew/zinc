# Display, GPU and session bus

## Wayland identity

Graphical containers normally mount a private `wp_security_context_v1` socket.
Before app connection Zinc supplies `sandbox_engine: com.github.crispuscrew.zinc`,
stable `app_id` across instances/restarts and runtime `instance_id` (`zcr where`).
The listener binds before registration; the compositor keeps accepting after that
connection closes. A holder retains revocation until app exit, then closes/removes
the socket. Podman mounts wait for holder readiness.

| `DisplayMeta` choice | Result |
| --- | --- |
| Default | Warn and use raw socket only if compositor lacks the protocol |
| `RequireSecurityContext: true` | Refuse downgrade; rejected for VMs (no guest bridge) |
| `DisableSecurityContext: true` | Explicit passthrough; cannot combine with Require |

Other setup errors fail launch; `zinc.wayland` records the actual result.
Identity is authenticated; **compositor policy decides permissions**. Tagging cannot
promise denial of every privileged operation; the container kernel boundary is separate.

## GPU and dimensions

GPU access is opt-out: `DisplayMeta.DisableGpuAccess: true`. `/dev/dri` adds driver
attack surface and exhaustion risk; no enforced `ResourcesMeta` VRAM cap exists.
dmem needs driver-registered regions/usable limits; controller presence proves no cap.
Venus `hostmem` is address space and framebuffer size follows resolution, neither a quota.
See [VM hardware](vm-hardware.md) for dimension/Vulkan validation.

## Filtered D-Bus

Empty `DBusMeta` grants nothing. `Talk` permits calls; `Own` permits claimed names.
Trailing `.*` includes future subnames. `KeepUserID` is required for socket access.
`xdg-dbus-proxy` runs outside the app pod/process in a separate helper; only it holds
the raw bus. Apps receive filtered instance sockets; unresolved requested host bus fails.
No VM guest bus bridge exists. Build with `make -C container/runner netfilter-image`,
select via `ZINC_NETFILTER_IMAGE`; the helper also applies nft in provisioned namespaces,
but creates no host topology.

## Attribution

`zcr bus --json` maps live proxy host PIDs to `app@instance`; `zcr where APP --json`
reports paths/proxy/socket (no grant: no bus). Join the kernel-derived
`org.freedesktop.DBus.GetConnectionUnixProcessID` result to that table.
Attribution uses Zinc-created resources, not app-claimed `--own` names. Each client
opens an upstream connection: zero clients means none; several share a proxy PID.
Query live state: recycled PIDs make cached attribution unreliable.
Use `app@instance`; ambiguous dotted names prefer the store's whole-app-name match.

## Notifications

Nonzero `NotificationMeta` needs a grant to `org.freedesktop.Notifications`:
- `Disabled` refuses Notify; `Silenced` accepts/drops it.
- `UseCustomPrefix` applies `CustomPrefix` to summaries.
- `AllowedActions`, `AllowedLinks`, `AllowedProlonged` allow buttons, anchor markup,
  and timeouts beyond the ten-second clamp.

Zero installs no filter, unlike explicit denials. The filter precedes the bus proxy
because names cannot constrain bodies. Notify is re-encoded for D-Bus alignment;
other traffic/portal file descriptors relay unchanged. VMs reject notifications.

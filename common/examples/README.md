# Canonical schema v4 examples

These are authoring examples, not a provisioner or a ready-made host setup.
`make -C common test` strictly decodes every YAML example without migration and
validates it. The broken fixture must decode, then report its specified errors.
JSON examples are checked against DNS and VM-options types as well.

| Example | Intent and prerequisites |
| --- | --- |
| `apps/firefox.yaml` | Offline browser; creates its image user; requires Wayland security context and deployed Zinc WirePlumber policy for native PipeWire playback. |
| `apps/hollywood.yaml` | Offline terminal with a derived Debian image and a created non-root user. |
| `apps/attached-shell.yaml` | Multiple terminals on one read-only container with writable, size-limited scratch space. Stop explicitly when done. |
| `apps/network-client.yaml`, `apps/network-server.yaml` | Reciprocal TCP grants; both need provisioned manifests and exact peer identities. Dependency ordering does not test HTTP readiness. |
| `apps/domain-web.yaml` | Explicit local DNS-proxy access and a destination IP snapshot from `example.com`; replace documentation addresses and provision the proxy. |
| `apps/guest.yaml`, `runtime/vm/guest.json` | Offline VM; replace the image path in both files and the all-zero placeholder pin with an independently approved disk digest. |
| `apps/firefox-broken.yaml` | Deliberate validation failures; never copy as a launch template. |
| `dns/transports.json` | Strict DNSMeta JSON demonstrating all five transports; documentation endpoints only. TCP/UDP entries explicitly permit plaintext fallback. |

Copy app YAML to `$XDG_CONFIG_HOME/zinc/apps/` and VM options to
`$XDG_CONFIG_HOME/zinc/runtime/vm/` only after adapting and reviewing them.
Default `XDG_CONFIG_HOME` is `~/.config`. Third-party container bases must already
be pulled by their exact digest; `Install` builds may need package-repository
access. Installed packages are not made reproducible merely by pinning the base.

Network rules never create links, routes, listeners, DNS aliases or services.
Declared interfaces require mandatory packet-preserving provisioning; there is
no automatic rootless pasta fallback. Empty interfaces mean no managed NIC.
See [network provisioning](../../docs/network-provisioning.md).

PipeWire grants need the host owner's opt-in
[policy deployment](../../integration/wireplumber/README.md). PulseAudio-only
clients need a separately restricted frontend; the raw host Pulse socket is not
provided. Named PipeWire and ALSA device selectors are host-specific.
The Firefox image account uses UID 1000 with KeepUserID; adapt that account UID
to the launching user's UID so the private broker socket remains accessible.

VM validation cannot establish that a disk exists, its pin matches, or the guest
supports its firmware, serial console and read-only choices. See
[VM architecture](../../docs/virtualization.md) before launching.

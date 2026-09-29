# Canonical schema v4 examples

Authoring fixtures require adaptation and host provisioning before launch.
`make -C common test` strictly decodes every YAML example without migration and
validates it; the broken fixture must report its specified errors. JSON is checked
against DNS/VM-options types. These checks do not prove host or guest readiness.

- `apps/firefox.yaml`: offline browser; creates an image user. Requires Wayland security
  context and deployed WirePlumber policy for native PipeWire. Match image UID 1000 to
  your UID for `KeepUserID` and private-socket access.
- `apps/hollywood.yaml`: offline Debian terminal with a created non-root user.
- `apps/attached-shell.yaml`: shared read-only container with bounded writable scratch;
  stop explicitly when done.
- `apps/network-{client,server}.yaml`: reciprocal TCP; provision both manifests and exact
  peer identities. Dependency ordering does not establish HTTP readiness.
- `apps/domain-web.yaml`: explicit proxy and `example.com` IP snapshot; replace documentation
  addresses and provision the proxy. No host DNS fallback.
- `apps/guest.yaml` + `runtime/vm/guest.json`: offline VM; replace both image paths and the
  zero pin with an independently approved digest. Validation cannot check disk existence/pin
  or guest firmware, serial-console and read-only boot support.
- `apps/firefox-broken.yaml`: deliberate failures, never a launch template.
- `dns/transports.json`: five transports, documentation endpoints; TCP/UDP allow plaintext fallback.

After review, copy YAML to `$XDG_CONFIG_HOME/zinc/apps/` and VM options to
`$XDG_CONFIG_HOME/zinc/runtime/vm/` (default `~/.config`). Pull container bases by exact
digest first; `Install` may need repository access. Base pinning does not pin installed packages.

Declared interfaces require [packet-preserving provisioning](../../docs/network-provisioning.md):
rules create no links/routes/listeners/DNS/services; there is no pasta fallback.
Empty interfaces mean no managed NIC. See [VM requirements](../../docs/virtualization.md).

PipeWire needs owner-approved [policy deployment](../../integration/wireplumber/README.md).
Named PipeWire/ALSA selectors are host-specific; PulseAudio-only clients need a separate
restricted frontend, never the raw host Pulse socket.

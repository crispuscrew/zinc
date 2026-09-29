# Quickstart: Zinc v0.11.0

This checkout prepares v0.11.0 (schema v4); it is not a published release.
v0.10.1 binaries use the earlier format. Upgrading? Read the
[upgrade requirements](releases/0.11.0.md#upgrade-requirements) first.

## Build and select binaries

Use rootless Podman and synchronized module vendors. From the repository root:

```sh
make -C creator build
make -C container/runner build
make -C launcher/tui build
export PATH="$PWD/creator/bin:$PWD/container/runner/bin:$PWD/launcher/tui/bin:$PATH"
zc version
zcr version
```

Optional: `make -C virtualization/runner build`, `make -C launcher/gui build`.
[Build details](build-and-checks.md) cover pinned Go 1.26.6, vendors and Nix.

## Start an offline shell

`zc init` preserves existing definitions unless forced. Fetch the approved base
first: launches use `--pull never`.

```sh
podman info
zc init
podman pull docker.io/library/alpine@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1
export ZINC_TERMINAL=foot       # select a terminal installed on your system
zc validate example-shell
zc run example-shell           # plan; no application process starts
zc run example-shell --exec
```

The shell has no NIC/audio and denies GPU access. Inspect with `zcr ps`; close
normally or run `zc stop example-shell`. `zlt` picks from the same app store.

## Add capabilities explicitly

Adapt [canonical examples](../common/examples/README.md), then
`zc validate APP --resolved` and inspect the plan. Binaries alone supply no host setup:

| Capability | Prerequisite |
| --- | --- |
| `NetworkMeta.Interfaces` | [Packet-preserving manifest](network-provisioning.md); rules create no links, routes, DNS aliases or publications; no automatic pasta |
| DNS | [Provisioned proxy](dns.md), addresses/control socket and explicit allow rule; runners verify live configuration |
| PipeWire | Explicit [WirePlumber deployment](../integration/wireplumber/README.md); native PipeWire is not PulseAudio |
| ALSA | Exact existing devices; see [audio](audio.md) |
| VM | Reviewed external base pin, QEMU/KVM, suitable firmware/hardware; [no guest agent or graphical Background](virtualization.md) |

More contracts: [architecture index](architecture.md).

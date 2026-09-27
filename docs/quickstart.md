# Quickstart: current canonical checkout

This guide uses schema v4 and current source-built tools. Tagged v0.10.1 release
binaries implement their release's format; they are not a way to validate these
new examples. See [builds/checks](build-and-checks.md) for the pinned Go 1.26.6
toolchain, vendor maintenance and Nix consumer builds.

## Build and select binaries

With rootless Podman available and module vendors synchronized:

```sh
make -C creator build
make -C container/runner build
make -C launcher/tui build
```

From the repository root, select those builds in this shell:

```sh
export PATH="$PWD/creator/bin:$PWD/container/runner/bin:$PWD/launcher/tui/bin:$PATH"
zc version
zcr version
```

Optional tools are `make -C virtualization/runner build` and
`make -C launcher/gui build`. Installing binaries alone does not provision
networking, configure QEMU/KVM, or deploy the audio policy.

## Start an offline shell

`zc init` preserves existing definitions unless explicitly forced. Zinc launches
with `--pull never`, so fetch the exact approved base before the first run:

```sh
podman info
zc init
podman pull docker.io/library/alpine@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1
export ZINC_TERMINAL=foot       # select a terminal installed on your system
zc validate example-shell
zc run example-shell           # plan; no application process starts
zc run example-shell --exec
```

The shell declares no NIC or audio grant and denies GPU access. While it is
open, `zcr ps` shows it. Close it normally or use `zc stop example-shell`.
`zlt` opens the picker over the same app store.

## Add capabilities explicitly

- Copy/adapt [canonical examples](../common/examples/README.md), then run
  `zc validate APP --resolved` before inspecting its launch plan.
- Adding `NetworkMeta.Interfaces` requires a matching
  [packet-preserving manifest](network-provisioning.md). Rules do not create
  links, routes, DNS aliases or host publications. No automatic pasta path exists.
- Configured DNS needs a provisioned local proxy and an explicit rule permitting
  it. Both runners provide [the DNS proxy command](dns.md) and verify its live
  configuration before launch; the provisioner supplies its addresses and socket.
- PipeWire audio needs an explicitly deployed
  [Zinc WirePlumber policy](../integration/wireplumber/README.md). ALSA-only
  grants need exact existing devices. Native PipeWire and PulseAudio are not
  interchangeable client protocols.
- VM launch needs a reviewed base pin in external options, suitable firmware,
  QEMU/KVM and guest-compatible hardware. Read [VM limits](virtualization.md),
  especially the absence of a guest agent and graphical Background support.

The [architecture index](architecture.md) links each contract and its limits.

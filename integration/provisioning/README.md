# Production container provisioning integration

This exercises the actual `container/runner/bin/zcr`, the canonical version-4
schema, on-disk provisioner manifests, rootless Podman, and real nftables.
It requires no Python packages, image pulls, installed services, or audio setup.
The verified positive result and adapter fixes are recorded in
[RESULTS.md](RESULTS.md); the original failure is retained in [BLOCKED.md](BLOCKED.md).

## Run

Run as the ordinary rootless Podman user, outside `podman unshare`:

```sh
make -C container/runner build BUILD_FLAGS='--pull=never --no-cache' \
  BUILD_IMAGE=localhost/zinc/provisioning-build-UNIQUE:local
make -C integration/provisioning lint test
```

Replace `UNIQUE` with an unused identifier. Remove only that test build tag/image
afterward. The build uses the existing pinned, offline Makefile entry point.
The harness records the tested binary's SHA-256 and checks that the runner's
vendored schema matches `common/domain/schema/schema.go` byte for byte.

Prerequisites: local `localhost/zinc/e2e-app:local` and
`localhost/zinc/netfilter:local`, Python 3 with Linux pidfd support, Podman,
`unshare`, `ip`, `nft`, and at least 1 GiB free in each working filesystem.
`/tmp/opencode` must already exist. Both image references are resolved locally
to immutable manifest digests. Production teardown currently uses the default
helper tag; before/after image inventories detect tag changes.

## Isolation and evidence

- `podman unshare` supplies the owning user namespace; its `unshare --net`
  child holds a separate network namespace until cleanup.
- Before provisioning, the harness requires different user/net inodes from the
  caller and only the new namespace's loopback device.
- The holder creates only `dummy0`, MAC `02:00:00:00:00:31`, address
  `10.203.0.1/32`, with ARP and IPv6 address generation disabled. Only kernel
  local/multicast routes are present. No host link, gateway, listener, published
  port, or external interface is configured.
- Owner-only manifests bind `/proc/<holder>/ns/{net,user}` to observed inodes.
  The typed app has one declared NIC, no allow rules, no DNS, and no audio.
  It runs as `nobody`, with a read-only root filesystem.
- Unique app/pod names are checked before cleanup is armed. Cleanup removes
  only those names and anonymous helpers referencing the holder's exact netns
  path. A test-local subreaper supervises/reaps daemonized descendants.
- Every attempt retains private evidence under the printed
  `/tmp/opencode/zinc-provisioning-*` directory: app/manifest, command output,
  namespace snapshots, launch plan, inspections, nft policy, and final result.
- Before/after inventories compare host links, addresses (excluding DHCP
  countdowns), routes/rules, namespace identities, and Podman containers, pods,
  networks, volumes, and images. Drift fails the test; unrelated resources are
  never rolled back. Host nft visibility is recorded separately: permission
  denial is not evidence that the host ruleset was read or compared.

## Required positive result

`make test` must exit zero only after all of these happen through production:

1. Explicit app filepath validation and real manifest `LoadFile`/`Resolve`.
2. `zcr run <absolute-filepath> --exec`: the NET_ADMIN helper installs policy in
   the designated namespace, then the pod joins the manifest's net namespace.
   A verified match to Podman's current userns selects `--userns host`; a distinct
   provisioned userns retains `--userns ns:<path>`. Both inodes remain mandatory.
3. The app reports matching namespace inodes, UID 65534, zero capability sets,
   and `NoNewPrivs: 1`; the observed nft table has the active default-deny policy.
   Live `zcr net <absolute-filepath> --json` reads that policy's counters. App/pod
   inspection must show no published ports.
4. Natural exit with status zero triggers supervisor removal of app/pod and
   replacement with the three-chain deny-all policy, preserving the topology.
5. A second launch with the same name/manifest also exits and cleans up.
6. A third, long-running launch is stopped by `zcr stop <absolute-filepath>`;
   app/pod disappear, policy closes, and outer inventories remain unchanged.

Failures exit nonzero. Assertions cannot be disabled via Python optimization.
The userns join diagnostic is read-only and never counts as a lifecycle pass:
it repeats the failing explicit join, then tests rootless Podman's current
userns while proving both namespace identities before reading nft.

## VM TAP provisioning

The separate VM target exercises the real `zvr` production path with a blank
disk, without installing a guest OS. See [VM_RESULTS.md](VM_RESULTS.md) for the
positive QMP/backend observations and cleanup results.

```sh
make -C virtualization/runner build BUILD_FLAGS='--pull=never --no-cache' \
  BUILD_IMAGE=localhost/zinc/provisioning-vm-build-UNIQUE:local
make -C integration/provisioning lint test-vm
```

Run outside `podman unshare`, as the ordinary rootless user. Additional tools:
`qemu-system-x86_64`, `qemu-img`, `nsenter`, `setpriv`, and usable `/dev/net/tun`.
The holder creates only persistent `tap0` in its private netns, owned by uid 0
of the owning rootless userns, so capability-dropped QEMU can attach it.
TUN denial fails the target and retains the exact holder stderr.

The fixture matches the existing VM offline harness: 16 MiB blank qcow2 base,
independently checked `zvr pin`, 128 MiB RAM, one vCPU, BIOS, read-only root disk,
headless display, and exact raw flags `-machine q35,accel=tcg -cpu max`. The
manifest uses topology `tap`, explicit MAC/address, no peers/publications/DNS,
and no uplink. No cloud-init, TPM, or audio is configured. The private TAP has
`10.203.0.1/32`; no guest IP configuration or guest OS boot is claimed.

The test checks real QMP `info network` and `info qtree`, both process namespace
inodes, all capability sets, memory, read-only block state, and production
`zvr net` counters. It launches three times using the same TAP: QMP `quit`,
manual `zvr stop --force`, then relaunch and another manual stop. Every exit
must terminate QEMU and its supervisor, remove runtime state, and close the
inet policy. The netdev ingress guard may remain until the namespace dies.

Private TAP comparison excludes only carrier/operstate transitions and the
kernel-generated local `ff00::/8` multicast route; raw snapshots are retained.
Configured identity, MAC, address, other routes, persistence, and all outer
inventory comparisons remain checked. Test-owned disks/logs remain as evidence;
the holder, TAP namespace, QEMU, and supervisors are released on exit.

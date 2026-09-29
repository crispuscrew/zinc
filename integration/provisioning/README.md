# Production container provisioning integration

Real `container/runner/bin/zcr`, schema v4, on-disk manifests, rootless Podman/nftables.
Evidence: [RESULTS.md](RESULTS.md) (historical pass/fixes), [BLOCKED.md](BLOCKED.md) (original failure).

## Run

From repository root, as the ordinary rootless Podman user outside `podman unshare`:

```sh
make -C container/runner build BUILD_FLAGS='--pull=never --no-cache' \
  BUILD_IMAGE=localhost/zinc/provisioning-build-UNIQUE:local
make -C integration/provisioning lint test
```

Replace `UNIQUE` with an unused identifier; afterward remove only that build tag/image.
Pinned offline builds; harnesses record binary SHA-256 and require byte-identical vendored `common/domain/schema/schema.go`.

- Requires local `localhost/zinc/e2e-app:local` and `localhost/zinc/netfilter:local`, Python 3 with Linux pidfd
  support, Podman, `unshare`, `ip`, `nft`, existing `/tmp/opencode`, set `XDG_RUNTIME_DIR`.
  Free space: 1 GiB each at repository/evidence/runtime; VM checks repository/evidence.
- No extra Python packages, image pulls, service installation or audio setup. Container image references resolve
  locally to digests; teardown uses the default helper tag, monitored by image inventories.

## Isolation and evidence

- `podman unshare` owns the userns; `unshare --net` child holds a netns until cleanup.
  Initial user/net inodes must differ from caller's; netns must be loopback-only.
- Holder adds only `dummy0`, MAC `02:00:00:00:00:31`, `10.203.0.1/32`; disables ARP/IPv6 address generation.
  Only kernel local/multicast routes; no host link, gateway, listener, publication or external interface.
- Owner-only manifests bind `/proc/<holder>/ns/{net,user}` to observed inodes. App: one NIC, no allows/DNS/audio,
  `nobody`, read-only rootfs. Check name uniqueness before cleanup; remove only test app/pod and exact-netns helpers.
  Test-local subreaper supervises/reaps descendants.
- Private `/tmp/opencode/zinc-provisioning-*` retains app/manifest, commands, snapshots, plan, inspections, nft/result.
  Compare host links, addresses (minus DHCP countdowns), routes/rules, namespaces and Podman containers/pods/networks/volumes/images.
  Drift fails; no unrelated rollback. nft permission denial does not prove host ruleset inspection/comparison.

## Required positive result

`make test` requires all production steps:

1. Validate explicit app filepath and manifest through `LoadFile`/`Resolve`.
2. `zcr run <absolute-filepath> --exec`: NET_ADMIN helper installs policy before pod netns join.
   Verified Podman-current userns uses `--userns host`; distinct provisioned userns uses `--userns ns:<path>`;
   both namespace inodes remain mandatory.
3. App proves matching inodes, UID 65534, zero capabilities, `NoNewPrivs: 1`; nft shows active default-deny.
   `zcr net <absolute-filepath> --json` reads live counters; app/pod inspection shows no publications.
4. Natural zero exit removes app/pod and installs three-chain deny-all, preserving topology.
5. Same-name/manifest relaunch exits/cleans up; third long-running launch ends via `zcr stop <absolute-filepath>`.
   App/pod disappear, policy closes, outer inventories stay unchanged.

Failures exit nonzero; Python optimization is refused. [Read-only userns diagnostics](diagnose.py) are not lifecycle passes.

## VM TAP provisioning

Real `zvr`, blank disk, no guest OS: historical [QMP/backend/cleanup evidence](VM_RESULTS.md).

```sh
make -C virtualization/runner build BUILD_FLAGS='--pull=never --no-cache' \
  BUILD_IMAGE=localhost/zinc/provisioning-vm-build-UNIQUE:local
make -C integration/provisioning lint test-vm
```

- Same user/build-tag rules. Also requires `qemu-system-x86_64`, `qemu-img`, `nsenter`,
  `setpriv`, usable `/dev/net/tun`. Persistent private `tap0` belongs to owning userns UID 0 for
  capability-dropped QEMU attachment. TUN denial fails; exact holder stderr retained.
- Offline fixture: 16 MiB blank qcow2, independently checked `zvr pin`, 128 MiB RAM, one vCPU, BIOS,
  read-only root disk, headless, `-machine q35,accel=tcg -cpu max`. Topology `tap`, explicit MAC,
  `10.203.0.1/32`; no peers/publications/DNS/uplink/cloud-init/TPM/audio. No guest IP or OS boot claim.
- Check QMP `info network`/`info qtree`, both process namespace inodes, all capability sets, memory,
  read-only blocks and `zvr net` counters. Reuse TAP across QMP `quit`, `zvr stop --force`, relaunch/stop.
  Exits terminate QEMU/supervisor, remove runtime state, close inet policy; ingress guard may persist until netns exit.
- Compare TAP identity/MAC/address/routes/persistence and outer inventories; exclude only carrier/operstate
  and kernel local `ff00::/8`. Retain raw snapshots/disks/logs in `/tmp/opencode/zinc-vm-provisioning-*`;
  release holder, TAP namespace, QEMU and supervisors on exit.

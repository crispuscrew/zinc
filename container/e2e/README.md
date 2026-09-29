# Zinc end-to-end tests

Black-box tests drive real `zc`/`zcr` binaries against rootless Podman.
Requires host Podman and Go; binaries are built with pinned container tooling.

```sh
make -C container/e2e e2e  # from repository root
```

The harness rebuilds both binaries, the nft helper and `localhost/zinc/e2e-app:local`.
For live-common workspaces, supply fresh `ZINC_E2E_CREATOR`/`ZINC_E2E_RUNNER` binaries.
Existing test-named containers/pods cause refusal before cleanup is registered.

Coverage:
- Authoring/validation; delegated `zc run --exec`, `ps`, `logs`, `stop`.
- `DependsOn` ordering, not service readiness.
- Attached `run`/`term` reopen the same holder with edited env maps; the headless
  terminal shim removes only TTY allocation.
- Resource limits, non-root user, raw swap argv, filtered D-Bus sockets/proxy attribution.
- Observed filtered/isolated attachment and packet counters across both endpoints;
  reciprocal policy can reject traffic before it reaches the producer.

Live network tests allow TCP/5432 and deny 9999. They require
`ZINC_E2E_NETWORK_MANIFEST_DIR` with matching producer/consumer manifests, interface
`link` and producer address `10.203.0.2`; see [provisioning](../../docs/network-provisioning.md).

Tests create no host topology. Without manifests, live networking skips; missing-topology
fail-closed coverage still runs. Passing without those fixtures does not verify live policy.

`*_test.go` orchestrates; `*.sh` files are in-image app entrypoints using `/bin/sh -c`.

# Zinc end-to-end tests

Black-box tests that drive the real `zc` and `zcr` binaries against rootless podman and
assert the guarantees unit tests cannot: that an app actually runs, and that the network
lock-down is actually enforced.

```
make e2e
```

Requires podman and a host `go` toolchain. The harness runs on the host because it
orchestrates real containers through the host's podman; the tools themselves are still
built in their pinned containers. The test rebuilds the two binaries, the
nft helper image, and a small `localhost/zinc/e2e-app:local` image, then runs its
scenarios, among them:

- **authoring** - `zc new` writes an app file and `zc validate` accepts it.
- **lifecycle** - `zc run --exec` (delegating to `zcr`) launches an app; `ps`, `logs`, and `stop` work.
- **network** - canonical From/To rules allow port 5432 and deny 9999. Requires
  `ZINC_E2E_NETWORK_MANIFEST_DIR` with matching owner-provisioned producer/consumer
  manifests, interface ID `link`, and producer address `10.203.0.2`. Tests never
  create host topology. Without manifests the live network scenario explicitly skips;
  a separate test always checks that missing topology fails closed.
- **counters** - observed attachment distinguishes filtered and isolated apps.
  Accepted and denied packets are counted across both endpoints because reciprocal
  policy may drop a packet before it reaches the producer.
- **dependencies** - `DependsOn` starts a dependency; it does not install a readiness probe.
- **attached reopen** - real Podman sessions receive edited env maps through both `run`
  and `term`, using the same live holder. A headless emulator shim removes only TTY allocation.
- **containment** - resource limits, a non-root user and explicit raw swap argv reach Podman.
- **desktop bus** - filtered sockets, proxy separation and host-bus PID attribution.

For live-common workspaces, set `ZINC_E2E_CREATOR` and `ZINC_E2E_RUNNER` to freshly
built binaries from that workspace. The default remains a rebuild through each tool's
Makefile. Existing test-named containers/pods cause refusal before test cleanup is registered.

The orchestration is split among `*_test.go`. The `*.sh` files are baked into the test
image as app entrypoints - the app's own behavior inside the container, not test logic -
and are invoked using the same `/bin/sh -c` entrypoint grammar as attached sessions.

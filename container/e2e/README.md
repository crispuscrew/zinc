# Zinc end-to-end tests

Black-box tests that drive the real `zc` and `zcr` binaries against rootless podman and
assert the guarantees unit tests cannot: that an app actually runs, and that the network
lock-down is actually enforced.

```
make e2e
```

Requires podman and a host `go` toolchain. The harness runs on the host because it
orchestrates real containers through the host's podman; the tools themselves are still
built in their pinned containers. The test builds anything missing (the two binaries, the
nft helper image, and a small `localhost/zinc/e2e-app:local` image), then runs its
scenarios, among them:

- **authoring** - `zc new` writes an app file and `zc validate` accepts it.
- **lifecycle** - `zc run --exec` (delegating to `zcr`) launches an app; `ps`, `logs`, and `stop` work.
- **tier-2 network** - a consumer reaches a producer's published port over a private link and is dropped on an unpublished one.
- **counters** - `zcr net` tells a filtered app from an isolated one, and the producer's own
  nftables counters show the two packets the previous scenario moved: one accepted on the
  published port, and the refusals on the unpublished one.

The orchestration is all Go (`e2e_test.go`). The `*.sh` files are baked into the test
image as app entrypoints - the app's own behavior inside the container, not test logic -
because the runtime passes an app's `Entrypoint` as a single-token executable.

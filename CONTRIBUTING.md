# Contributing to Zinc

Start with the [README](README.md) and [architecture index](docs/architecture.md).

## Golden rules

1. **Functional core, imperative shell.** Keep schema, validation and policy builders pure;
   put I/O and process execution in adapters or CLI/TUI edges.
2. **Pinned Podman tooling through `make`.** Tool `test`, `vet`, `fmt`, `vendor` and `build`
   run in the digest-pinned Go container. Run built binaries, never `go run`.
   Host Go and `go.work` are only for development iteration and the E2E harness.
3. **Branch before committing.** Never commit, amend or push directly to the default branch;
   use reviewed PRs and the [protected-branch release flow](RELEASES.md#cutting-a-release).
4. **Minimum but sufficient.** Small complete changes; no speculative abstractions.
5. **Descriptive identifiers, at least 3 characters.** Only exception: `t *testing.T`.
6. **[Image trust](docs/images-and-mounts.md).** Third-party images require `@sha256:` + 64 hex;
   only `localhost/` permits mutable tags. Local derived images inherit their pinned base.
7. **Protected schema.** Do not edit `common/domain/schema/schema.go`, including formatting,
   without explicit owner permission. Report any new format-gate failure instead.
8. **Plain punctuation.** No em dash or section sign in code, docs, output or commits.

## Repo layout and the zc/zcr split

See [components and dependency direction](docs/components.md#repository-and-dependency-direction).
`zc` and launchers delegate to `zcr`/`zvr` as subprocesses, never import a runner;
shared schema/contracts live in `common`, while each runner owns its backend.

### The runner hexagon (`container/runner`)

Dependencies point inward: `domain` is pure; `ports` defines contracts; `app` orchestrates;
`adapters` implements I/O; `wire` composes them. Replace mechanisms through ports.
Network, bus and display brokers prepare before app startup and clean up after exit.

## Build, test, validate

In each touched module, use `make help`, then **`make check`** (format check, vet, tests).
Tool modules use `make build` and `make repro`; behavior changes need tests and relevant
[integration/E2E checks](docs/build-and-checks.md#examples-and-integration).
`make -C container/e2e e2e` needs real Podman; skips do not prove enforcement.

`make vendor` is the networked, `GOWORK=off` dependency refresh. Refresh common and affected
consumer vendors after shared changes; review module/vendor changes and run `vendor-check`
(also mutating). See [toolchain, commands and Nix](docs/build-and-checks.md).

## Security model (read before touching launch/network/image/mount/cap code)

- Single-user, rootless host; escalation includes ungranted host access or egress-allowlist escape.
- Baseline: `--security-opt no-new-privileges --cap-drop all`. Validate added caps, devices,
  mounts and sockets in `common/domain/schema/validate`; shared YAML is partly untrusted.
- [Packet-preserving provisioning](docs/network-provisioning.md) locks policy before startup;
  review manifest trust, both endpoints and spoofing/bypass constraints. No automatic topology/pasta fallback.
- [DNS worker/readiness](docs/dns.md) is wired in both runners; runtime deployment is still required.
  PipeWire needs opt-in [WirePlumber policy](integration/wireplumber/README.md).
- Raw backend argv can override typed guarantees. Wayland identity depends on compositor policy;
  [VM guest-side bridges](docs/virtualization.md#shared-fields-stop-at-the-guest-boundary) are unavailable.

## Documentation map

[Components/tool references](docs/components.md) | [Roadmap](ROADMAP.md) |
[Release plan](RELEASES.md) | [Changelog](CHANGELOG.md)

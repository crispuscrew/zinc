# Contributing to Zinc

Guidance for contributing to this repo. It is the portable,
in-repo companion to the [architecture index](docs/architecture.md) and
[`README.md`](README.md). Canonical field declarations live in `common/domain/schema`.

Zinc is a security-focused sandboxing core. Every user-facing app runs via a **rootless
Podman container** (primary runtime) or a **QEMU VM**. Desktop grants have explicit
limits: Wayland identity depends on compositor policy, and VM guest-side bridges
are unavailable. Runtime prerequisites are separate from installation. Priority order:
**Stable, then Secure, then Beautiful.**

## Golden rules

1. **Podman-only tool builds. No host Go for the binaries.** Every Go command for a tool
   (`test`, `vet`, `fmt`, `vendor`, `build`) runs inside the digest-pinned `golang`
   container, invoked through `make`. There is no `go run`: `make build` produces a binary
   in the container; you run that binary. (The dev `go.work` and the host `go` are only for
   fast local iteration and the end-to-end harness.)
2. **Branch before you commit.** Never commit, amend, or push directly to the
   default branch: branch off it first.
3. **Minimum but sufficient.** Make the smallest change that fully solves the task. No
   speculative abstraction, no gold-plating.
4. **Descriptive variable names, at least 3 letters** (`cmd` not `c`, `idx` not `i`). The
   only exception is `t *testing.T`.
5. **[Image trust](docs/images-and-mounts.md).** Third-party images must be pinned by a
   canonical digest (`@sha256:` + 64 hex). Only `localhost/` images may use a mutable tag.
   Derived images (`FROM image` + the install layer) are local and inherit the pinned base.
6. **No em dash and no section sign in any output**, including code comments, docs, and
   commit messages. Use a plain hyphen for punctuation; write "section 5.3" or name the
   thing instead of the section glyph.

## Repo layout and the zc/zcr split

- `common/domain` - pure schema, validation, migration, inheritance and runtime policy types.
- `common/adapters` - shared network, DNS and audio I/O; common now includes pinned DNS/QUIC dependencies.
- `container/runner` (**zcr**) - the runtime. It reads an app file and runs it via rootless
  podman, applying the network lock-down. It is a ports-and-adapters hexagon (below).
- `creator` (**zc**) - the authoring tool (CLI + keyboard-first TUI). It depends
  ONLY on `common` and shells out to whichever runner binary on `$PATH` owns the app - `zcr`
  for container apps, `zvr` for VM apps. It never imports a runner; they meet only at the
  on-disk YAML format.
- `container/e2e` - black-box end-to-end tests that drive the real binaries against podman.
- `virtualization/runner` - `zvr`, the VM runner. Same split as the container side: it
  depends only on `common` and drives `qemu-system-x86_64` directly.

App files are YAML at `~/.config/zinc/apps/<name>.yaml`. zc's keybind config is under
`~/.config/zinc/zc`.

### The runner hexagon (`container/runner`)

Keep the dependency direction inward:

| Package | Role | Rule |
|---|---|---|
| `domain` | schema-derived types + derived-image policy | no I/O (no podman, fs, nft, env) |
| `ports` | interfaces (`Store`, `Runtime`, `ImageBuilder`, `ImageResolver`, `NetEnforcer`, `DBusBroker`, `DisplayBroker`) + the neutral `Command` type | contracts only |
| `app` | launch orchestration (`Service`) | depends on ports + domain |
| `adapters/{podman,netenforce,dbusproxy,waylandctx,fs,host}` | the I/O implementations | implement ports |
| `wire` | composition root helpers | assembles adapters |

`NetEnforcer` is the network swap point in `adapters/netenforce`: swapping the mechanism is
a new adapter, not a cross-cutting edit. `DBusBroker` (`adapters/dbusproxy`) and
`DisplayBroker` (`adapters/waylandctx`, the Wayland security context) are its siblings for the
session bus and the display, same shape and the same reason: a capability the app must never
hold directly, established before the app exists and removed after it dies.

## Build, test, validate

Each module shares one build pipeline ([`check.mk`](check.mk) + [`tool.mk`](tool.mk) + one
digest-pinned [`Containerfile`](Containerfile)). Work from a module directory:

```sh
cd container/runner    # or creator, common
make check             # gofmt + go vet + go test, in the pinned container
make build             # reproducible build, produces ./bin/<tool>
make vendor            # refresh vendored deps (the only networked step; GOWORK=off)
make netfilter-image   # (container runner) helper for namespace nft policy and D-Bus
```

The gate before declaring work done is **`make check` green in every module you touched**.
The end-to-end suite (`make -C container/e2e e2e`) and CI (`.github/workflows/ci.yml`) run
the two tools plus the podman-backed scenarios.

Toolchain: Go 1.26.6-alpine, pinned by digest in `check.mk`/`Containerfile`.
Host e2e Go must be supported and >=1.26.6. The common manifest declares Go 1.26;
consumer `make vendor` refreshes lift their directives as needed. During this
transition vendors may lag canonical sources: use explicit temporary overlays
for canonical checks and report the distinction. Preserve owner-controlled
`schema.go` formatting; report its format-gate failure rather than rewriting it.
See [build/check details](docs/build-and-checks.md), including Nix compatibility.

## Security model (read before touching launch/network/image/mount/cap code)

- Single-user, rootless host. "Privilege escalation" means a container gaining
  capability/host-access it was not granted, or **escaping its egress allowlist** - not
  root-on-host.
- **Networked apps require packet-preserving provisioning.** The dedicated namespace
  is locked before app startup. Adapters create no host topology and offer no
  automatic pasta fallback. Review manifest trust, both endpoints' policy and
  spoofing/bypass constraints in [network provisioning](docs/network-provisioning.md).
- Baseline is least privilege: `--security-opt no-new-privileges --cap-drop all`. Anything
  re-adding capability or host access (caps, devices, mounts, sockets) is an attack-surface
  decision - validate it in the validator (`common/domain/schema/validate`).
- Configs are **partly untrusted** (shared, distributed as examples), so trust/audit
  controls must hold up to a reviewer reading the YAML.
- Raw backend argv is a warned override of typed guarantees. PipeWire grants
  require opt-in WirePlumber deployment. DNS CLI/readiness integration is still
  pending; do not document these prerequisites as automatically satisfied.

## Documentation map

- [`docs/architecture.md`](docs/architecture.md) - concise index with a mapping from
  historical section numbers to focused current pages.
- [`creator/README.md`](creator/README.md) and
  [`container/runner/README.md`](container/runner/README.md) - per-tool docs.
- [`README.md`](README.md) - overview and quickstart.
- [`ROADMAP.md`](ROADMAP.md) and [`RELEASES.md`](RELEASES.md) - what is planned and the
  release plan.

# AGENTS.md - working in Zinc

Guidance for AI agents (and humans) contributing to this repo. It is the portable,
in-repo companion to [`docs/architecture.md`](docs/architecture.md) (the single source of
truth) and [`README.md`](README.md).

Zinc is a keyboard-first, security-focused app-sandboxing core. Every user-facing app runs
in a **rootless Podman container** (primary) or a libvirt/qemu VM. Priority order, always:
**Stable, then Secure, then Beautiful.**

## Golden rules

1. **Podman-only tool builds. No host Go for the binaries.** Every Go command for a tool
   (`test`, `vet`, `fmt`, `vendor`, `build`) runs inside the digest-pinned `golang`
   container, invoked through `make`. There is no `go run`: `make build` produces a binary
   in the container; you run that binary. (The dev `go.work` and the host `go` are only for
   fast local iteration and the end-to-end harness.)
2. **Commit only when the user explicitly asks.** Do not commit, amend, or push on your own
   initiative. When asked, branch off the default if you are on it.
3. **Minimum but sufficient.** Make the smallest change that fully solves the task. No
   speculative abstraction, no gold-plating.
4. **Descriptive variable names, at least 3 letters** (`cmd` not `c`, `idx` not `i`). The
   only exception is `t *testing.T`.
5. **Image trust (architecture section 5.5).** Third-party images must be pinned by a
   canonical digest (`@sha256:` + 64 hex). Only `localhost/` images may use a mutable tag.
   Derived images (`FROM image` + the install layer) are local and inherit the pinned base.
6. **No em dash and no section sign in any output**, including code comments, docs, and
   commit messages. Use a plain hyphen for punctuation; write "section 5.3" or name the
   thing instead of the section glyph.

## Repo layout and the zcc/zcr split

- `common/` - the shared library: the app-config schema and its validation, pure stdlib.
  Both the creator and the runner depend on it and nothing else shared.
- `container/runner` (**zcr**) - the runtime. It reads an app file and runs it via rootless
  podman, applying the network lock-down. It is a ports-and-adapters hexagon (below).
- `container/creator` (**zcc**) - the authoring tool (CLI + keyboard-first TUI). It depends
  ONLY on `common` and shells out to the `zcr` binary on `$PATH` to run apps. It never
  imports the runner; the two meet only at the on-disk YAML format.
- `container/e2e` - black-box end-to-end tests that drive the real binaries against podman.
- `launcher/` and `virtualization/creator/` exist as skeletons but do NOT compile yet (they
  still reference the removed `core` module); they are on the roadmap.

App files are YAML at `~/.config/zinc/apps/<name>.yaml`. zcc's keybind config is under
`~/.config/zinc/zcc`.

### The runner hexagon (`container/runner`)

Keep the dependency direction inward:

| Package | Role | Rule |
|---|---|---|
| `domain` | schema-derived types + derived-image policy | no I/O (no podman, fs, nft, env) |
| `ports` | interfaces (`Store`, `Runtime`, `ImageBuilder`, `ImageResolver`, `NetEnforcer`) + the neutral `Command` type | contracts only |
| `app` | launch orchestration (`Service`) | depends on ports + domain |
| `adapters/{podman,netenforce,fs,host}` | the I/O implementations | implement ports |
| `wire` | composition root helpers | assembles adapters |

`NetEnforcer` is the network swap point in `adapters/netenforce`: swapping the mechanism is
a new adapter, not a cross-cutting edit.

## Build, test, validate

Each module shares one build pipeline ([`check.mk`](check.mk) + [`tool.mk`](tool.mk) + one
digest-pinned [`Containerfile`](Containerfile)). Work from a module directory:

```sh
cd container/runner    # or container/creator, common
make check             # gofmt + go vet + go test, in the pinned container
make build             # reproducible build, produces ./bin/<tool>
make vendor            # refresh vendored deps (the only networked step; GOWORK=off)
make netfilter-image   # (runner) build the nft lock-down helper image once
```

The gate before declaring work done is **`make check` green in every module you touched**.
The end-to-end suite (`make -C container/e2e e2e`) and CI (`.github/workflows/ci.yml`) run
the two tools plus the podman-backed scenarios.

## Security model (read before touching launch/network/image/mount/cap code)

- Single-user, rootless host. "Privilege escalation" means a container gaining
  capability/host-access it was not granted, or **escaping its egress allowlist** - not
  root-on-host.
- **Network enforcement is the crown jewel.** An app must never see an unfiltered network,
  even briefly: the app's own netns is locked by nftables *before* the app process starts
  (fail-closed). Any open window, any way config can relax the ruleset, or any
  non-fail-closed path is high severity.
- Baseline is least privilege: `--security-opt no-new-privileges --cap-drop all`. Anything
  re-adding capability or host access (caps, devices, mounts, sockets) is an attack-surface
  decision - validate it in the validator (`common/domain/schema/validate`).
- Configs are **partly untrusted** (shared, distributed as examples), so trust/audit
  controls must hold up to a reviewer reading the YAML.

For a security pass, use the project-local **`secops-reviewer`** agent
([`.claude/agents/secops-reviewer.md`](.claude/agents/secops-reviewer.md)) - it has this
threat model baked in and reviews only (never edits).

## Documentation map

- [`docs/architecture.md`](docs/architecture.md) - single source of truth. Its section
  numbers are cited in the code (for example section 5.5 image trust, section 5.3 the
  network lock-down).
- [`container/creator/README.md`](container/creator/README.md) and
  [`container/runner/README.md`](container/runner/README.md) - per-tool docs.
- [`README.md`](README.md) - overview and quickstart.
- [`ROADMAP.md`](ROADMAP.md) and [`RELEASES.md`](RELEASES.md) - what is planned and the
  release plan.
- [`llms.txt`](llms.txt) - this doc set as a link index for LLM tooling.

# AGENTS.md — working in HyprZinc

Guidance for AI agents (and humans) contributing to this repo. It is the portable,
in-repo companion to [`docs/architecture.md`](docs/architecture.md) (the single
source of truth) and [`README.md`](README.md).

HyprZinc is a keyboard-first, security-focused Hyprland desktop. Every user-facing
app runs in a **rootless Podman container** (primary) or a libvirt/qemu VM. Priority
order, in this order, always: **Stable → Secure → Beautiful.**

## Golden rules

1. **Podman-only build. There is no host Go.** Every Go command — `test`, `vet`,
   `fmt`, `vendor`, `build` — runs inside the digest-pinned `golang` container,
   invoked through `make`. Never run `go` on the host, and there is no `go run`:
   `make build` produces a binary in the container; you run that binary.
2. **Commit only when the user explicitly asks.** Do not commit, amend, or push on
   your own initiative. When asked, branch off the default if you're on it.
3. **Minimum but sufficient.** Make the smallest change that fully solves the task.
   No speculative abstraction, no gold-plating.
4. **Descriptive variable names, ≥3 letters** (`cmd` not `c`, `idx` not `i`). The
   only exception is `t *testing.T`.
5. **Image trust (§5.5).** Third-party images must be pinned by a canonical digest
   (`@sha256:` + 64 hex). Only locally built `trusted-*` images may use a local tag.
   Derived images (`FROM image` + `RUN install`) are local and inherit the pinned
   base.

## Build, test, validate

Each tool is its own Go module (`core/`, `hzc/`, `hzl/`, `hzv/`) sharing one build
pipeline ([`check.mk`](check.mk) + [`tool.mk`](tool.mk) + one
[`Containerfile`](Containerfile)). Work from a module directory:

```sh
cd hzc                 # or core / hzl / hzv
make check             # gofmt-check + go vet + go test, in the pinned container
make build             # reproducible build → ./bin/<tool>   (not core)
make vendor            # refresh vendored deps (the only networked step; GOWORK=off)
make help              # list every target
```

The gate before declaring work done is **`make check` green in every module you
touched** — at minimum `core`, and `hzc`/`hzl` if they consume the change (`hzl`
wires the app service). `hzv` imports no core and is usually unaffected.

hzc-specific helpers:

```sh
cd hzc
make validate APP=examples/apps/firefox.toml   # build, then validate a config
make run RUN_ARGS="list"                        # build, then run a subcommand
make netfilter-image                            # build the nft egress helper image once
```

## Layout — the core hexagon

`core/` is a ports-and-adapters hexagon. Keep the dependency direction inward:

| Package | Role | Rule |
|---|---|---|
| `core/domain` | pure model, validation, presets, derived-image policy | **no I/O** — no podman, fs, nft, or env |
| `core/ports` | interfaces (`Store`, `Runtime`, `ImageBuilder`, `ImageResolver`, `NetEnforcer`) + the neutral `Command` type | contracts only |
| `core/app` | launch orchestration (`Service`) | depends on ports + domain |
| `core/adapters/{podman,netenforce,fs,host}` | the I/O implementations | implement ports |
| `core/wire` | composition root helpers | assembles adapters |

`NetEnforcer` is the egress swap point: `none` / `pasta` / `container` adapters live
in `adapters/netenforce`. Swapping the mechanism = a new adapter, not a
cross-cutting edit. `core` is vendored into `hzc` and `hzl` via `replace => ../core`;
`go.work` is dev-only.

## Security model (read before touching launch/network/image/mount/cap code)

- Single-user, rootless host. "Privilege escalation" means a container gaining
  capability/host-access it wasn't granted, or **escaping its egress allowlist** —
  not root-on-host.
- **Egress enforcement is the crown jewel.** A `pasta` app must never see an
  unfiltered network, even briefly: the pod's netns is locked by nftables *before*
  the app container starts (fail-closed). Any open-egress window, any way config can
  relax the ruleset, or any non-fail-closed path is high severity.
- Baseline is least privilege: `--security-opt no-new-privileges --cap-drop all`.
  Anything re-adding capability or host access (caps, devices, mounts, sockets) is an
  attack-surface decision — validate it in `domain/validate.go`.
- Configs are **partly untrusted** (shared, distributed as examples, Nix-seeded), so
  trust/audit controls must hold up to a reviewer reading the TOML.

For a security pass, use the project-local **`secops-reviewer`** agent
([`.claude/agents/secops-reviewer.md`](.claude/agents/secops-reviewer.md)) — it has
this threat model baked in and reviews only (never edits).

## Documentation map

- [`docs/architecture.md`](docs/architecture.md) — single source of truth (§ numbers
  are cited throughout the code, e.g. §5.5 image trust, §5.3 egress, §9.1 terminals).
- [`man/hzc.1`](man/hzc.1) — the `hzc` CLI (`man -l man/hzc.1`).
- [`man/hyprzinc-app.5`](man/hyprzinc-app.5) — the app-definition TOML format
  (`man -l man/hyprzinc-app.5`).
- [`README.md`](README.md) — overview and quickstart.
- [`ROADMAP.md`](ROADMAP.md) — milestones (currently M3, network egress).
- [`llms.txt`](llms.txt) — this doc set as a link index for LLM tooling.

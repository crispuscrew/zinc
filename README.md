# HyprZinc

Keyboard-first, security-focused Hyprland desktop. Everything user-facing runs in
rootless Podman containers (primary runtime) or libvirt/qemu VMs (heavy isolation),
wired together by a Nix home-manager flake.

**Priority order: Stable → Secure → Beautiful.**

- Architecture (single source of truth): [`docs/architecture.md`](docs/architecture.md)
- Roadmap: [`ROADMAP.md`](ROADMAP.md)

## Components

Each tool name is **h**ypr**z**inc + its domain:

| Tool  | Expands to              | Role                            | Stack         |
|-------|-------------------------|---------------------------------|---------------|
| `hzp` | Podman         | container-definition TUI        | Go, Bubbletea |
| `hzl` | Launcher       | launcher + smart executor (GUI) | Go, gioui     |
| `hzv` | Virtualization | VM manager TUI                  | Go, Bubbletea |

## Status

**M1 — hzp container lifecycle** (see the roadmap). Implemented: the app-config
schema, pure validation (incl. the §5.5 image-trust rule), presets, the pure podman
runspec builder, a config store under `~/.config/hyprzinc/apps`, and real container
lifecycle (`run/stop/restart/inspect/logs`) — all behind the `hzp` CLI. `hzl` and
`hzv` exist as buildable skeletons so all three tools share one module layout and
build pipeline; their UIs land in M7 / M9.

## Develop

Each tool is its own Go module in its own directory (`hzp/`, `hzl/`, `hzv/`). They
share **one** set of build logic via [`tool.mk`](tool.mk) and **one** generic,
digest-pinned [`Containerfile`](Containerfile) — "the same logic, only different
paths." Each tool's `Makefile` is three lines: set `TOOL`, then `include
../tool.mk`.

Work from any tool's directory:

```sh
cd hzp                 # or hzl / hzv
make check             # gofmt-check + go vet + go test
make container-build   # build reproducibly in the pinned container → ./bin/<tool>
make repro             # prove the container build is byte-identical across runs
make help              # list every target
```

`hzp` adds config-specific helpers on top of the shared set:

```sh
cd hzp

# define an app (saved to ~/.config/hyprzinc/apps), then inspect/launch it
go run . new firefox --image docker.io/library/firefox@sha256:… --preset strict
go run . list
go run . validate firefox
go run . run firefox          # prints the podman command (dry-run)
go run . run firefox --exec   # actually launches it (rootless)
go run . logs firefox -f
go run . restart firefox
go run . stop firefox

# a bare name resolves against the store; a path is read directly:
go run . validate examples/apps/firefox.toml
```

Dependencies are vendored per module and the Go toolchain is pinned by digest, so
`make container-build` is hermetic: same inputs → same bytes, on any machine, with
no network at compile time. [`.gitattributes`](.gitattributes) marks `vendor/` as
vendored, so GitHub keeps it out of language stats and collapses it in diffs.

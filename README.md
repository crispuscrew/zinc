# Zinc

**Zinc** is a keyboard-first, security-focused **sandboxing core**. It runs
user-facing apps in rootless Podman containers (primary runtime) or libvirt/qemu VMs
(heavy isolation), each walled off from the rest of the desktop through the Wayland
security-context protocol. Zinc is compositor-agnostic and installs cleanly on any
existing system.

**ZDE** (Zinc Desktop Environment, `zde`) is the full environment built on Zinc,
shipped in two variants — `zde-niri` and `zde-hypr` — wired together by a Nix
home-manager flake.

**Priority order: Stable → Secure → Beautiful.**

- Architecture (single source of truth): [`docs/architecture.md`](docs/architecture.md)
- Roadmap: [`ROADMAP.md`](ROADMAP.md)
- Manual pages: [`man/hzc.1`](man/hzc.1) (the CLI) · [`man/hyprzinc-app.5`](man/hyprzinc-app.5)
  (the app TOML format) — read them without installing: `man -l man/hzc.1`
- For AI agents / LLM tools: [`AGENTS.md`](AGENTS.md) (how to work in this repo) ·
  [`llms.txt`](llms.txt) (doc index)

## Components

Every tool is `zinc-<kind>-<role>` — `<kind>` is `container` or `virtualization`,
`<role>` is `creator` or `runner` — plus the `zinc-launcher-<ui>` picker. The short
code is the initials.

| Short | Tool                          | Role                                  |
|-------|-------------------------------|---------------------------------------|
| `zcc` | `zinc-container-creator`      | define container apps (write configs) |
| `zcr` | `zinc-container-runner`       | launch + supervise a container app    |
| `zvc` | `zinc-virtualization-creator` | define VM apps                        |
| `zvr` | `zinc-virtualization-runner`  | launch + supervise a VM app           |
| `zlg` | `zinc-launcher-gui`           | fast app launcher (GUI)               |
| `zlt` | `zinc-launcher-tui`           | fast app launcher (TUI)               |

A **creator** defines an app and writes its config; a **runner** is what actually
starts that app and owns its lifecycle; **launchers** are quick pickers over the
defined apps. They all share one library — [`common/`](common), the app schema +
domain logic — so container and VM apps use the same config format.

Layout: `common/`, `container/{creator,runner}`, `virtualization/{creator,runner}`,
`launcher/`.

## Status

**M3 — Network egress enforcement** (see the roadmap). Implemented: the app-config
schema, pure validation (incl. the §5.5 image-trust rule), presets, the pure podman
runspec builder, a config store under `~/.config/hyprzinc/apps`, real container
lifecycle, a keyboard-first Bubbletea TUI, and **per-app egress enforcement** —
`pasta` apps launch in a locked-down pod whose netns an nft init step filters to the
configured CIDR/port allowlist (`block_dns`, modes `none`/`pasta`) *before* the app
starts. vpn-container routing is next (M4). `hzl` and `hzv` exist as buildable
skeletons so all three tools share one module layout and build pipeline; their UIs
land in M7 / M9.

## Develop

The shared core — a **hexagon** (ports & adapters) — lives in the [`core/`](core)
module: `domain/` (pure schema + validation + policy), `ports/` (interfaces),
`app/` (the launch orchestration), `adapters/` (podman, the `netenforce` egress
strategies, fs, host), and `wire/` (composition root). `hzc` and `hzl` both consume
it (via a local `replace => ../core`, vendored in) and drive everything through the
one `app.Service`, so an app launches through the exact same code path no matter
which front-end starts it. Swapping a mechanism — e.g. the egress enforcer behind
the `NetEnforcer` port — is a new adapter, not a cross-cutting edit.

Each tool is its own Go module in its own directory (`hzc/`, `hzl/`, `hzv/`). They
share **one** set of build logic: [`check.mk`](check.mk) (containerized
test/vet/fmt/vendor, included by every module incl. `core`) plus [`tool.mk`](tool.mk)
(binary build/run/repro) and **one** generic, digest-pinned
[`Containerfile`](Containerfile) — "the same logic, only different paths." A tool's
`Makefile` is three lines: set `TOOL`, then `include ../tool.mk`; `core`'s just
`include ../check.mk`.

This is a **podman-only** workflow: there is no host Go. Every Go command — test,
vet, fmt, vendoring, and the build — runs inside the digest-pinned `golang`
container, invoked through `make`. There is no `go run`; `make` builds the binary
in the container and you run that.

Work from any tool's directory:

```sh
cd hzc                 # or hzl / hzv / core
make check             # gofmt-check + go vet + go test, all in the pinned container
make build             # build reproducibly in the pinned container → ./bin/<tool>  (not core)
make repro             # prove the container build is byte-identical across runs
make vendor            # refresh vendored deps (the only step that needs network)
make help              # list every target
```

`hzc` drives the host's podman, so you build it in the container and run the
**produced binary** (`./bin/hzc`). Call the binary directly, or let `make` build
then run it via `make run RUN_ARGS="…"`:

```sh
cd hzc
make build                     # → ./bin/hzc, built in the pinned container

# keyboard-first manager: create / edit / run / stop / logs, no mouse
./bin/hzc tui

# find an image without a browser, and pin a tag to its digest (§5.5):
./bin/hzc image search alpine
./bin/hzc image resolve alpine:3.20   # → docker.io/library/alpine@sha256:… (paste into app.image)

# scriptable CLI (a bare name resolves against the store; a path is read directly):
./bin/hzc new firefox --image docker.io/library/firefox@sha256:… --preset strict
./bin/hzc list
./bin/hzc run firefox --exec   # launch rootless · run (no --exec) prints the podman command
./bin/hzc logs firefox -f
./bin/hzc restart firefox
./bin/hzc stop firefox

# quick setup: app.install builds a derived image (FROM image + RUN install), §5.5
# e.g. base debian + install "apt-get update && apt-get install -y hollywood"
./bin/hzc build hollywood       # (re)build now · a plain run rebuilds on change too

# multiterminal app: open more terminals into one shared instance (§9.1)
./bin/hzc term devbox            # another terminal running the app's command
./bin/hzc term devbox --shell    # …or a shell into the same container

# TUI keybindings: pick a scheme (default | vim | your own), edit, validate
./bin/hzc keys list
./bin/hzc keys set vim
./bin/hzc keys edit vim          # scaffold a custom copy and open it in $EDITOR

make validate APP=examples/apps/firefox.toml   # build, then validate a config
make run RUN_ARGS="list"                        # build, then run any subcommand via make
```

**In the TUI (default scheme):** `n` new · `e` edit · `r` run · `s` stop · `l` logs ·
`d` delete · `?` keybind schemes · `q` quit. In a form: `tab`/`↑↓` move, `←/→/space`
change a value, `ctrl+d` clear a field, `ctrl+r` resolve the image to a pinned digest,
`ctrl+s` save, `esc` cancel; the **advanced** row opens the full TOML in `$EDITOR`
(where `[keys]`/`[[mounts]]`/caps live).

**Remappable keys.** These are hzc's *own* TUI keys (not Hyprland hotkeys). They
resolve through a selectable scheme: `default`, `vim`, or a custom
`~/.config/hyprzinc/hzc/schemes/<name>.toml` (a `base` to inherit + a partial
`[bindings.<screen>]` override). Choose one with `hzc keys set`, or press `?` in the
TUI for the live picker. hzc's defaults are already vim-friendly, so the two built-ins
differ only modestly — custom schemes are where the flexibility is.

**Multiterminal apps.** A terminal app with `multiterminal = true` runs a shared
keep-alive container (`sleep infinity` under `--init`) that many terminals attach to
with `podman exec` — each running the app's `command` or a shell. The container lives
until the **last** terminal closes (unless `background = true` keeps it up). Open more
with `hzc term <name>` or, in the TUI, **run** again (or **shell**). Lifecycle is
ref-counted by filesystem flock under `$XDG_RUNTIME_DIR/hyprzinc/run/` — no daemon.
It needs `sleep`/`/bin/sh` in the image and an explicit `command`.

**Pasta (egress-filtered) apps** launch inside a podman pod whose netns is locked
to the config's CIDR/port allowlist by an nft init step before the app starts
(§5.3). Build the nft helper image once: `make netfilter-image`.

Dependencies are vendored per module and the Go toolchain is pinned by digest, so
`make container-build` is hermetic: same inputs → same bytes, on any machine, with
no network at compile time. [`.gitattributes`](.gitattributes) marks `vendor/` as
vendored, so GitHub keeps it out of language stats and collapses it in diffs.

# Zinc

**Zinc** is a keyboard-first, security-focused **sandboxing core**. It runs user-facing
apps in rootless Podman containers (primary runtime) or libvirt/qemu VMs (heavy
isolation), each walled off from the rest of the desktop through the Wayland
security-context protocol. Zinc is compositor-agnostic and installs cleanly on any
existing system.

**ZDE** (Zinc Desktop Environment, `zde`) is a separate project built on Zinc - the full
environment, shipped in two variants (`zde-niri` and `zde-hypr`) wired together by a Nix
home-manager flake, and developed in its own repository.

**Priority order: Stable, then Secure, then Beautiful.**

- Architecture: [`docs/architecture.md`](docs/architecture.md)
- Roadmap: [`ROADMAP.md`](ROADMAP.md) and releases: [`RELEASES.md`](RELEASES.md)
- For AI agents / LLM tools: [`AGENTS.md`](AGENTS.md)

## Components

Every tool is `zinc-<kind>-<role>`: `<kind>` is `container` or `virtualization`, `<role>`
is `creator` or `runner`, plus the `zinc-launcher-<ui>` picker. The short code is the
initials.

| Short | Tool                          | Role                                  | Status  |
|-------|-------------------------------|---------------------------------------|---------|
| `zcc` | `zinc-container-creator`      | define container apps (write configs) | 0.1     |
| `zcr` | `zinc-container-runner`       | launch + supervise a container app    | 0.1     |
| `zvc` | `zinc-virtualization-creator` | define VM apps                        | planned |
| `zvr` | `zinc-virtualization-runner`  | launch + supervise a VM app           | planned |
| `zlg` | `zinc-launcher-gui`           | fast app launcher (GUI)               | 0.3     |
| `zlt` | `zinc-launcher-tui`           | fast app launcher (TUI)               | 0.2     |

A **creator** defines an app and writes its config; a **runner** actually starts that app
and owns its lifecycle; **launchers** are quick pickers over the defined apps. They share
one library, [`common/`](common) (the app schema + validation), so container and VM apps
use the same config format.

The creator carries no runtime: `zcc` authors app files and shells out to the `zcr` binary
to run them, so the two meet only at the on-disk YAML format and never share code.

Layout: `common/`, `container/{creator,runner}`, `container/e2e` (end-to-end tests), and
`launcher/{common,tui,gui}` (the shared launcher library and the TUI/GUI pickers).

## Status

**0.1 - containers.** Implemented: the app-config schema and validation (including the
rule that third-party images must be digest-pinned), a YAML config store under
`~/.config/zinc/apps`, real rootless-container lifecycle, a keyboard-first Bubbletea TUI,
and the **fail-closed network lock-down** applied in the app's own network namespace
before it starts:

- no network lists: the app reaches only its own localhost (isolated)
- egress list: default-drop, allow only the listed destination CIDRs and ports
- ingress publish: expose the app's own ports to the LAN, filtered by source
- sibling link: a private internal bridge between two apps, gated per-port

Not yet supported (rejected, not run): host-scoped egress, gateway / multi-homing, and
combining a sibling link with other networking on one app. The launcher and virtualization
tools are on the roadmap.

## Install

Podman-only, reproducible builds. Build the binaries and put them on your `$PATH`:

```sh
make -C container/runner build     # produces container/runner/bin/zcr
make -C container/creator build    # produces container/creator/bin/zcc
make -C launcher/tui build         # produces launcher/tui/bin/zlt  (0.2)
make -C launcher/gui build         # produces launcher/gui/bin/zlg  (0.3)
install -Dm755 container/runner/bin/zcr  ~/.local/bin/zcr
install -Dm755 container/creator/bin/zcc ~/.local/bin/zcc
install -Dm755 launcher/tui/bin/zlt      ~/.local/bin/zlt
install -Dm755 launcher/gui/bin/zlg      ~/.local/bin/zlg
```

`zcc` needs `zcr` on `$PATH` to run apps (authoring works without it). To run
egress-filtered apps, build the nft helper image once:

```sh
make -C container/runner netfilter-image
```

## Usage

```sh
# author with zcc: a bare name resolves against ~/.config/zinc/apps; a path is read directly
zcc new firefox --image docker.io/library/firefox@sha256:...
zcc list
zcc validate firefox
zcc tui                        # keyboard-first manager: create / edit / run / stop / logs

# find and pin an image (third-party images must be digest-pinned)
zcc image search alpine
zcc image resolve alpine:3.20  # gives docker.io/library/alpine@sha256:... to paste in

# run: zcc forwards these to zcr. run without --exec prints the podman plan first
zcc run firefox --exec
zcc logs firefox -f
zcc stop firefox

zcc version

# launch with zlt (0.2): a keyboard-first fuzzy picker over your apps
zlt                            # open the picker: type to filter, enter launches, esc quits
zlt firefox                    # or launch one directly (bind this to a desktop hotkey)

# launch with zlg (0.3): the same picker as a graphical window (pure-Go Wayland)
zlg                            # open the picker window: type to filter, enter launches
zlg firefox                    # or launch one directly (bind this to a desktop hotkey)
```

In the TUI (default scheme): `n` new, `e` edit, `r` run, `s` stop, `l` logs, `d` delete,
`R` rename, `?` keybind schemes, `q` quit. In a form: `tab`/arrows move, `space` toggles,
`ctrl+d` clears a field, `ctrl+r` resolves the image to a pinned digest, `ctrl+s` saves,
`esc` cancels; the **advanced** row opens the full YAML in `$EDITOR` (where capabilities,
network lists, volumes, and keys live).

TUI keys are zcc's own (not desktop hotkeys); they resolve through a selectable scheme
(`default`, `vim`, or a custom one under `~/.config/zinc/zcc`). Pick one with
`zcc keys set`, or press `?` for the live picker.

## Develop

The container runtime is a **hexagon** (ports and adapters) in
[`container/runner`](container/runner): `domain/` (schema-derived types), `ports/`
(interfaces), `app/` (launch orchestration), `adapters/` (podman, the `netenforce`
enforcers, fs, host), and `wire/` (composition). `zcc` depends only on `common` and shells
out to the `zcr` binary, so it never imports the runtime.

Podman-only: there is no host Go for the tool builds. Every Go command (test, vet, fmt,
vendor, build) runs inside a digest-pinned `golang` container via `make`. Work from any
module:

```sh
cd container/runner            # or container/creator, common
make check                     # gofmt + vet + test in the pinned container
make build                     # reproducible build, produces ./bin/<tool>
make repro                     # prove the build is byte-identical across runs
make vendor                    # refresh vendored deps (the only step that needs network)
```

The end-to-end tests drive the real binaries against podman:

```sh
make -C container/e2e e2e
```

Dependencies are vendored per module and the Go toolchain is pinned by digest, so
`make build` is hermetic: same inputs, same bytes, on any machine, with no network at
compile time.

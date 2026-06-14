# HyprZinc — Roadmap

Source of truth: [`docs/architecture.md`](docs/architecture.md). Priority order
**Stable → Secure → Beautiful**.

**Style:** functional core / imperative shell — schema, validation, and the
podman/VM "runspec" builders are pure functions over decoded data; I/O and process
execution live at the edges (CLI/TUI). Every milestone ships with tests and a
runnable exit check.

Legend: ✅ done · 🚧 in progress · ⬜ not started

---

## M0 — Foundation ✅
Repo skeleton (§13), the **app-config schema** + pure **validation**, and the pure
**podman runspec builder** (`AppConfig → podman run` argv). Thin `hzp` CLI:
`validate`, `run` (dry-run by default, `--exec` to launch).

Each tool is its own Go module (`…/hyprzinc/{hzp,hzl,hzv}`) with vendored deps;
all three share one build pipeline — a repo-root `tool.mk` + a generic, digest-
pinned `Containerfile` (`make container-build`). `hzp` is implemented; `hzl`/`hzv`
are buildable skeletons. The pure packages live in `hzp/internal/{config,runspec}`
— they are the functional core and **graduate to a standalone `…/hyprzinc/core`
module when `hzl`/`hzv` need them** (§13). No second consumer yet ⇒ no multi-module
plumbing yet.
- **Exit:** `hzp validate examples/apps/firefox.toml` passes; `hzp run …` prints the
  correct `podman` command; `go test ./...` green.

## M1 — hzp container lifecycle ✅
Config store CRUD under `~/.config/hyprzinc/apps/`; presets as templates (§4);
save- **and** launch-time validation; real `run/stop/restart/inspect/logs` via
podman; digest-pin (third-party) vs local-tag (`trusted-*`) handling (§5.5).
- **Exit:** define + launch firefox in a rootless container with strict defaults;
  stop / restart / logs work.

## M2 — hzp TUI (Bubbletea) ⬜
Keyboard-first create / edit / delete / launch / stop; preset picker that shows each
field's **actual value**, not just the label (§4); logs view; "save current running
state as profile."
- **Exit:** manage apps end-to-end without leaving the TUI or touching a mouse.

## M3 — Network egress enforcement ⬜
pasta wiring; **nftables-in-netns** CIDR + port allowlist (§5.3); `block_dns`;
modes `none` / `pasta`.
- **Exit:** a `pasta` app reaches only allowlisted CIDRs/ports; 53/853 blocked except
  the designated resolver.

## M4 — vpn-container ⬜
Image: sing-box + amnezia-wg + xray + entrypoint that renders `config.json` from
home-manager input (§6.1); socks5 backends; destination-CIDR routing; **fail-closed
DNS** (§6.5); `network.mode = "container"` attach; `depends_on` ordering (§6.6).
- **Exit:** app routed through vpn-container with per-destination backend selection;
  VPN down ⇒ no resolution, no leak.

## M5 — Trusted image layering ⬜
Containerfiles `trusted-base → trusted-go → trusted-go-dev` (+ `trusted-rust`) (§7);
`FROM` pinned **by digest**, package versions pinned; local build flow; nvim only in
`-dev`.
- **Exit:** `trusted-go-dev` builds; an app references a locally-built trusted image.

## M6 — Theme bundle, audio, keys, mounts ⬜
Curated **theme bundle** RO mount + theme env vars (§5.6); pipewire socket /
`legacy_alsa`; ssh/gpg key mounts + agent sockets + 0600 enforcement (§3 `[keys]`);
general `[[mounts]]`.
- **Exit:** a containerized GTK/Qt app matches the host theme; audio + ssh-agent work
  on explicit grant only.

## M7 — hzl launcher + smart executor (gioui) ⬜
`Super+G`; fuzzy search across apps / built-in commands / projects / custom commands /
VMs (§9.2); vertical card list; icon store + 64×64 thumbnail cache (§9.2); built-in
commands; custom-command TOML; project scan paths; `depends_on` auto-start.
- **Exit:** launch any source by keyboard from one search line.

## M8 — Nix home-manager module + flake ⬜
Flake outputs `hzp/hzl/hzv` + `homeManagerModules.hyprzinc` (§9.3); module generates
Hyprland config, binaries on PATH, terminal/shell/fonts, the **theme bundle**, and a
**first-run seed** of TOMLs (hzp owns them thereafter); vpn backends/routes;
`projectPaths`.
- **Exit:** `home-manager switch` brings a fresh workstation to full state.
  *(Blocked on this box — Nix not installed.)*

## M9 — hzv VM manager (MVP) ⬜
Bubbletea TUI; rootless `libvirt` user session + qemu; VM TOML schema (§10.3);
**`window` display mode only** (others return a clear "not implemented"); network
`user`/`bridge`/`none`; disposable (`destroy_on_shutdown`); user-provided base
images; hzl integration.
- **Exit:** define + launch a VM in `window` mode; disposable wipes on shutdown.

## M10 — Profiles, hotkeys, autostart ⬜
Profile config + activation (modal-confirm stop of apps not in the profile, start with
`depends_on`, workspace placement) (§8); Hyprland hotkeys (§12); login autostart
(`autostart`, `autostart_workspace`).
- **Exit:** `Super+P` profile switch reshapes the running session.

---

### Cross-cutting
- **Honesty:** where a mechanism is partial (wsc §5.2, GPU §5.4) say so in UI + docs.
- **Every change:** `go test ./...`, `go vet ./...`, gofmt clean.
- Known tradeoffs tracked in architecture §14.

### Dependency order
```
M0 → M1 → M2
        └→ M3 → M4
M5 ┄ feeds M1, M4
M6 ┄ after M1
M7 ┄ after M2 (+ M9 for the VM source)
M8 ┄ after the three tools build  (needs Nix)
M9 ┄ parallel to M7
M10 ┄ last
```

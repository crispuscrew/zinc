# Changelog

All notable changes to Zinc are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Zinc uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). The version line is
tracked in [RELEASES.md](RELEASES.md).

## [0.3.0] - unreleased

Adds the GUI launcher.

### Added

- **`zlg` (zinc-launcher-gui)** - the graphical sibling of `zlt`: the same quick
  picker over the defined apps, as a floating `wlr-layer-shell` overlay. Type to
  filter, up/down (or ctrl+p/n) to move, enter launches the selected app through
  `zcr` and quits, and a dot marks apps already running. `zlg <app>` launches one
  directly, for a desktop hotkey. The picker window itself is the new `menu` module
  (below), so `zlg` is a thin consumer that just supplies the app list and an activate
  callback; it inherits a **static, `CGO_ENABLED=0`, runs-anywhere, byte-reproducible**
  binary with no cgo and no graphics libraries. Like `zcc` and `zlt` it shells out to
  the `zcr` binary and never imports the runtime.
- **`launcher/common`** - a shared library holding the read-side app store, the `zcr`
  delegate, and the fuzzy matcher, so `zlt` and `zlg` share one copy of the
  list / launch / match logic (and its security guards).
- **`menu`** - a standalone, reusable overlay-menu module (repo-root `menu/`, module
  path `github.com/crispuscrew/zinc/menu`) extracted out of `zlg`: a pure-Go Wayland
  `wlr-layer-shell` overlay, a software renderer, a keymap, a theme resolver, and a
  fuzzy-filter picker view-model, all behind one call - `menu.Run(items, activate,
  opts)`. It speaks layer-shell directly through a hand-written binding and reads the
  system light/dark theme from the XDG portal, and it depends on **no** Zinc sibling
  module (Go `replace` directives are not transitive), so `zde` and a future wofi-like
  picker can import it too. The fuzzy matcher is copied in as `menu/internal/match`.
- **App grouping** - a new optional `Group` field on the app config (schema v2, additive)
  files apps into sections. The launcher shows a section header per group when idle and
  flattens to a plain ranked list as soon as you type. The demo apps ship with groups.
- **App icons** - the launcher now draws each app's icon in a left column. It resolves the
  existing `Icon` field (a freedesktop icon name or an absolute image path) by looking the
  name up in the icon-theme directories, decoding and scaling in pure Go. It is raster-only
  (PNG/JPEG/GIF); an SVG-only or missing icon just leaves that row's slot blank.
- **Font** - text is antialiased (rendered with `x/image/opentype`), not the old bitmap face.
  It auto-detects an installed system Nerd Font (monospace, so it matches your terminal),
  falling back to the bundled Go Mono when none is found. `ZLG_FONT=/path/to/font.ttf` pins a
  specific font.

### Known limitations

- `zlg`'s keymap is US-QWERTY; full keyboard-layout (xkb) support is future work.
- Like `zlt`, `zlg` lists and launches; managing an app (stop, logs, edit) stays in
  `zcc`.

## [0.2.0] - 2026-07-19

Adds the launcher.

### Added

- **`zlt` (zinc-launcher-tui)** - a fast, keyboard-first fuzzy picker over the
  defined apps. Type to filter (an in-house subsequence matcher that favours
  matches at the start of a name and at word boundaries), up/down (or ctrl+p/n) to
  move, enter launches the selected app through `zcr` and quits (dmenu-style), and
  a `●` marks apps already running (from `zcr ps`). `zlt <app>` launches one
  directly, for a desktop hotkey. Like `zcc` it depends only on `common` and
  shells out to the `zcr` binary, so it never imports the runtime - dependency
  auto-start, the network lock-down, and derived-image builds stay `zcr`'s job. It
  lives at `launcher/tui`, with `launcher/gui` reserved for the planned `zlg`.

### Known limitations

- `zlt` lists and launches; managing an app (stop, logs, edit) stays in `zcc`. The
  GUI launcher (`zlg`) and richer rows are on the roadmap.
- `virtualization/creator/` is still a non-compiling skeleton (0.7), excluded from
  the build and CI.

## [0.1.0] - 2026-07-16

First release. Ships the container tools: author an app once, run it sandboxed.

### Added

- **`common`** - the shared library: schema version 2 (a flat `AppConfig` with
  grouped `*Meta` structs) and pure validation, run identically at author time
  and at launch time. Third-party images must be digest-pinned.
- **`zcc` (zinc-container-creator)** - a keyboard-first Bubbletea TUI and a
  scriptable CLI to create, validate, edit, rename, and delete app files, with
  selectable keybind schemes (`default`, `vim`, or a custom one) and
  image search / digest-resolve. It depends only on `common` and shells out to
  the `zcr` binary to run apps.
- **`zcr` (zinc-container-runner)** - the rootless-podman runtime:
  `run` / `build` / `validate` / `stop` / `restart` / `inspect` / `logs` /
  `term` / `ps` / `image`. Derived images from `ImageMeta.Install`, terminal and
  multiterminal apps, dependency auto-start, and a dry run that prints the exact
  podman commands and nft ruleset.
- **Fail-closed network lock-down** - applied by nftables in the app's own
  network namespace before the app process starts, with no unfiltered window:
  isolated (localhost only), egress whitelist, LAN publish filtered by source,
  and per-port sibling links over a private internal bridge. A small
  digest-pinned netfilter helper image applies the ruleset.
- **Reproducible, podman-only builds** - a digest-pinned Go toolchain, vendored
  deps, and byte-identical output (`make repro`); a `version` stamped from
  `git describe`; a Go end-to-end suite against real podman; and CI.

### Known limitations

- The network model **rejects at launch** (fail-closed, never mis-enforced):
  host-scoped egress, gateway / multi-homing, and combining a sibling link with
  any other networking on one app.
- Schema-defined but not yet wired into the launch: `ResourcesMeta`,
  `InternalUserMeta`, `NotificationMeta`, and bundle-relative `Configs` mounts.
- `launcher/` and `virtualization/creator/` are skeletons that do not compile
  yet; they are on the roadmap and excluded from the build and CI.

[0.3.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.3.0
[0.2.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.2.0
[0.1.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.1.0

# Changelog

All notable changes to Zinc are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Zinc uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). The version line is
tracked in [RELEASES.md](RELEASES.md).

## [0.3.1] - 2026-07-25

Fixes the two things that made the 0.3 overlay look broken: a frozen window during a launch,
and a grid of thumbnails that never appeared.

### Changed

- **`zlg` no longer freezes while an app starts.** `menu`'s activate callback now runs off the
  Wayland event loop, so the overlay keeps drawing - with a `launching <app>...` banner - while
  `zcr` works, instead of sitting frozen on screen holding an exclusive keyboard grab for the
  whole launch (a second or two normally, minutes when a derived image has to be rebuilt).
  Esc dismisses the window immediately; the launch carries on in the background and `menu.Run`
  returns once it finishes, so no activation outlives the call. A second Enter while one launch
  is in flight is ignored rather than starting a duplicate. Callers name the banner's verb with
  the new `Options.BusyVerb`.
- **Thumbnail and icon decoding is bounded in bytes, not pixels.** The old guard capped declared
  pixels, which is not a memory bound: the same dimensions cost one byte per pixel as a paletted
  image and eight at 16 bits per channel, so a deep-colour file crafted to sit just under the
  pixel cap still decoded to several times what the cap implied. The budget is now the decoded
  size the file's own header implies, which cuts the grid's adversarial worst case from about
  1.7 GiB to 480 MiB while still accepting every plausible wallpaper (an 8K photograph decodes
  to about 132 MiB).

### Fixed

- **Grid thumbnails never appeared unless they decoded within the first 160ms.** The frame
  callback that paces the decode poll is double-buffered Wayland state, applied only when the
  surface commits, and the poll's "nothing landed yet, ask again" path asked without committing.
  The callback therefore never fired and the poll died silently, leaving every tile on its
  placeholder until the next keypress. It went unnoticed because the entrance fade commits a
  frame per refresh while it runs, which is long enough for the small sample images but not for
  a directory of real photographs.

### Known limitations

Unchanged from 0.3.0: the keymap is US-QWERTY, and grid thumbnails decode without
prioritisation or cancellation. The launch freeze is gone, but a launch still cannot be
**cancelled**: Esc dismisses the overlay and the launch runs to completion behind it.

## [0.3.0] - 2026-07-25

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
- **`menu` grid layout** - the `menu` module can lay items out as a thumbnail grid
  (`Options.Grid`) instead of the default list: each item's new `Preview` image is drawn as a
  tile with its label beneath, arrow keys navigate in two dimensions, and typing still
  fuzzy-filters. Thumbnails decode in the background (bounded, cgo-free, aspect-preserving) and
  appear as they land, so a directory of large images stays responsive. The decode/scale core
  is shared with the icon path in the new `internal/imgutil`, and WebP is now decodable too.
- **`wallpaper` example** - a thumbnail wallpaper chooser built on the grid layout
  (`menu/example/wallpaper`), alongside the `dmenu` example. It scans a directory, shows the
  images as a filterable grid, and on Enter sets the chosen one via `$WALLPAPER_CMD`
  (`swww`/`swaybg`/`hyprpaper`) or prints the path when that is unset - proving the grid is
  reusable by an ordinary program and staying compositor-agnostic.
- **`make -C menu wallpaper-demo`** - the grid's counterpart to the launchers' `demo` targets:
  generates sample wallpapers, builds the example, and opens the grid over them, so the layout
  can be tried in one command. `WALLPAPER_DIR=...` points it at your own images instead. The
  samples are generated deterministically (`menu/example/wallpaper/gen`) rather than committed,
  so the repository carries no binary image assets.
- **Screenshots** - the READMEs now show the launcher and the grid (`docs/media`), captured
  headlessly under sway so they are reproducible and contain nobody's desktop.

### Fixed

- **Double launch in `zlt`** - a second enter while a launch was still in flight started a
  second `zcr run` of the same app. The two raced on one pod, and the loser's fail-closed
  teardown removed the pod the winner had just created, killing the app while the picker
  still reported success. Enter is now ignored until the launch resolves.
- **App names containing `..` were unusable** - the store rejected any name with `..` as a
  substring, not just the traversal segment, so an app called `my..app` was listed by both
  launchers but could never be loaded or launched. The guard now checks path segments; the
  separator check that actually confines a name to the apps directory is unchanged.
- **App names ending in `.yaml` ran the wrong file** - `zcr` reads such an argument as a
  filesystem path, so launching the app `notes.yaml` ran whatever `./notes.yaml` happened to
  be in the launcher's working directory (for a hotkey, `$HOME`) instead of the stored app.
  The launchers now reject that name and point at the explicit `./notes.yaml` path form.

### Known limitations

- `zlg`'s keymap is US-QWERTY; full keyboard-layout (xkb) support is future work.
- Like `zlt`, `zlg` lists and launches; managing an app (stop, logs, edit) stays in
  `zcc`.
- **`zlg` freezes while an app starts.** `menu`'s activate callback is synchronous, so `zlg`
  runs `zcr run` on the Wayland event loop: the overlay stops redrawing and keeps its
  exclusive keyboard grab until `zcr` returns. That is a second or two for a normal launch,
  but minutes for an app whose derived image has to be rebuilt. An asynchronous activate is
  0.4 work.
- The grid decodes thumbnails for every cell it has drawn, with no prioritisation or
  cancellation, so scrolling fast through a very large directory makes the visible tiles
  queue behind ones already scrolled past.
- The thumbnail decode guard caps declared pixels, which bounds a plausible image but not a
  crafted one: a deep-colour file at the pixel limit still decodes to far more memory than
  the cap implies.

## [0.2.1] - 2026-07-22

Adds a runtime volume flag to `zcr`.

### Added

- **`zcr run --volume` / `-v`** - mount an extra host directory or file into an app
  for one run only, without editing its config:
  `zcr run <app> -v HOST:CONTAINER[:OPTIONS]` (repeatable). `OPTIONS` defaults to
  `ro,noexec`; `rw` makes it writable and `exec` executable. The runtime mount is
  appended to the loaded config in memory (never written back to the YAML) and passes
  through the same field-shift / injection validation and the same arg-builder as a
  configured `Volumes` entry. This delivers the "can also be added temporarily at
  runtime" behavior the `Volumes` schema field always described.

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

[0.3.1]: https://github.com/crispuscrew/zinc/releases/tag/v0.3.1
[0.3.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.3.0
[0.2.1]: https://github.com/crispuscrew/zinc/releases/tag/v0.2.1
[0.2.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.2.0
[0.1.0]: https://github.com/crispuscrew/zinc/releases/tag/v0.1.0

# zlg - zinc-launcher-gui

`zlg` is the graphical sibling of [`zlt`](../tui/README.md): the same quick picker over the
defined apps (`~/.config/zinc/apps`), as a floating Wayland overlay. Type to filter, move with
the arrows (or `ctrl+p`/`ctrl+n`), and press enter to launch the selected app; a dot marks
apps already running. `zlg <app>` launches one app directly, for a desktop hotkey.

```sh
zlg            # open the picker window: type to filter, enter launches, esc quits
zlg firefox    # launch a defined app directly (bind this to a hotkey)
zlg --version
```

Like `zcc` and `zlt`, `zlg` **never imports the runtime**: it lists what `zcc` authored and
shells out to the `zcr` binary to run the chosen app, so dependency auto-start, the network
lock-down, and derived-image builds all stay `zcr`'s job.

## A thin consumer of the `menu` module

The picker window itself is not in `zlg` anymore: the overlay core (a pure-Go Wayland
`wlr-layer-shell` surface, a software renderer, a keymap, a theme resolver, and the
fuzzy-filter picker view-model) was extracted into the standalone [`menu`](../../menu)
module, and `zlg` is now a thin consumer of it. `zlg` loads the defined apps, marks the ones
`zcr` reports running, and hands `menu.Run` an activate callback that launches the chosen app
through `zcr`; the window, rendering, input, and theming all live in `menu`. The old
`internal/picker`, `internal/keymap`, `internal/render`, and `internal/ui` packages moved
there.

## Pure Go, static, reproducible

Because it builds on `menu`, `zlg` renders without cgo: `menu` speaks the Wayland wire
protocol directly (via [go-wayland](https://github.com/rajveermalviya/go-wayland)) and
software-renders into a shared-memory buffer with a bundled bitmap font. So, unlike a GUI
toolkit that pulls in libwayland / EGL / GTK, `zlg` stays a **static, `CGO_ENABLED=0`,
runs-anywhere, byte-reproducible** binary built from the same minimal image as the other Zinc
tools. The read/launch/match logic is still shared with `zlt` through the
[`launcher/common`](../common) library.

## Build

```sh
make build        # reproducible static build -> ./bin/zlg
make check        # gofmt + vet + test in the pinned container
make repro        # prove the build is byte-identical
```

## Known limits (0.3)

- The keymap is US-QWERTY; full keyboard-layout (xkb) support is future work (the keymap
  lives in `menu`).
- It lists and launches; managing an app (stop, logs, edit) stays in `zcc`.

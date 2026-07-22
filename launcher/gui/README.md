# zlg - zinc-launcher-gui

`zlg` is the graphical sibling of [`zlt`](../tui/README.md): the same quick picker over the
defined apps (`~/.config/zinc/apps`), as a Wayland window. Type to filter, move with the
arrows (or `ctrl+p`/`ctrl+n`), and press enter to launch the selected app; a dot marks apps
already running. `zlg <app>` launches one app directly, for a desktop hotkey.

```sh
zlg            # open the picker window: type to filter, enter launches, esc quits
zlg firefox    # launch a defined app directly (bind this to a hotkey)
zlg --version
```

Like `zcc` and `zlt`, `zlg` **never imports the runtime**: it lists what `zcc` authored and
shells out to the `zcr` binary to run the chosen app, so dependency auto-start, the network
lock-down, and derived-image builds all stay `zcr`'s job.

## Pure Go, static, reproducible

`zlg` renders without cgo. It speaks the Wayland wire protocol directly (via
[go-wayland](https://github.com/rajveermalviya/go-wayland)) and software-renders the picker
into a shared-memory buffer with a bundled bitmap font. So, unlike a GUI toolkit that pulls
in libwayland / EGL / GTK, `zlg` stays a **static, `CGO_ENABLED=0`, runs-anywhere,
byte-reproducible** binary built from the same minimal image as the other Zinc tools.

The read/launch/match logic is shared with `zlt` through the
[`launcher/common`](../common) library. Everything except the Wayland event loop is a pure,
unit-tested package (`internal/picker`, `internal/keymap`, `internal/render`); only
`internal/ui` needs a live compositor.

## Build

```sh
make build        # reproducible static build -> ./bin/zlg
make check        # gofmt + vet + test in the pinned container
make repro        # prove the build is byte-identical
```

## Known limits (0.3)

- The keymap is US-QWERTY; full keyboard-layout (xkb) support is future work.
- `zlg` is a normal Wayland window, not a dmenu-style overlay - the pure-Go Wayland library
  carries no `wlr-layer-shell` yet.
- It lists and launches; managing an app (stop, logs, edit) stays in `zcc`.

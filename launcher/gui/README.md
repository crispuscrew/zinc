# zlg - zinc-launcher-gui

`zlg` picks apps from `$XDG_CONFIG_HOME/zinc/apps` (default `~/.config`) in a floating
Wayland overlay. It needs a compositor supporting `wlr-layer-shell`, such as niri,
Hyprland or sway. For a terminal picker, use [`zlt`](../tui/README.md).

```sh
zlg            # open the picker
zlg firefox    # launch directly, e.g. from a hotkey
zlg --version
make -C launcher/gui demo  # from repo root; uses throwaway config
```

Type to filter; arrows or `ctrl+p`/`ctrl+n` move, Enter launches, Esc quits.
A dot marks running apps (best effort). Launch requires `zcr` for containers or
`zvr` for VMs on `PATH`; see [demo prerequisites](../demo/README.md).

![The zlg launcher overlay](../../docs/media/zlg-launcher.png)

## Appearance and behaviour

Environment variables are the only appearance settings:

- `ZLG_OPACITY`: percentage (`20`) or fraction (`0.2`); `1`, `100` and unset are opaque.
  Invalid values are reported on stderr and ignored. The compositor must blend layer surfaces.
- `ZLG_FONT`: `.ttf`/`.otf` path; default is an installed monospace Nerd Font, then Go Mono.
- `ZLG_NO_ANIM`: any value disables entrance fade-in.
- `ZLG_DEBUG`: any value traces the Wayland handshake to stderr.

```sh
ZLG_OPACITY=85 zlg
ZLG_DEBUG=1 zlg
```

## A thin consumer of the `menu` module

[`menu`](../../menu/README.md) owns the window, input and rendering;
[`launcher/common`](../common) shares app loading, matching and runtime delegation with `zlt`.

## Pure Go, static, reproducible

The static `CGO_ENABLED=0` build uses pure-Go Wayland and software rendering.
Use `make repro` to check byte reproducibility in your checkout.

## Build

```sh
make build  # bin/zlg, pinned Podman tooling
make check  # formatting, vet, tests
make repro  # compare two builds
```

<a id="known-limits-03"></a>

## Known limits

- US-QWERTY keymap only; app management stays in `zc` and the runners.
- Launch runs off the event loop but cannot be cancelled. Esc closes the window;
  the process waits for the launch to finish, including slow image builds.

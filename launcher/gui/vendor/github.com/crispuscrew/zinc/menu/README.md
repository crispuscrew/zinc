# menu - a reusable Wayland overlay menu

Pure-Go fuzzy picker in a centered Wayland overlay. Requires `wlr-layer-shell`
(e.g. niri, Hyprland, sway). Builds with `CGO_ENABLED=0`, has no Zinc sibling
dependencies, and is used by [`zlg`](../launcher/gui/README.md).

## API

Import `github.com/crispuscrew/zinc/menu`; declarations are in [`menu.go`](menu.go).

```go
func Run(items []Item, activate ActivateFunc, opts Options) (int, error)
type ActivateFunc func(item Item) error

type Item struct {
    Label       string // primary text; fuzzy-match target
    Description string // dimmed secondary text
    Group       string // optional header; keep group items adjacent
    Icon        string // freedesktop name or absolute image path
    Preview     string // image path for a grid thumbnail
    Marked      bool   // indicator dot; caller-defined meaning
}

type Options struct {
    Prompt     string  // default "> "
    Footer     string  // default "up/down move   enter select   esc quit"
    AppID      string  // compositor namespace/app-id; default "menu"
    FontPath   string  // .ttf/.otf; empty keeps process default
    Width      int     // pixels; default 720
    Height     int     // pixels; default 440
    Opacity    float64 // 0..1; <= 0 means opaque
    NoAnim     bool    // disable entrance fade
    Debug      bool    // Wayland handshake to stderr
    BusyVerb   string  // activation banner verb; default "running"
    Grid       bool    // thumbnails instead of a list
    CellWidth  int     // grid only, pixels; default 180
    CellHeight int     // grid only, pixels; default 140
}
```

`Run` returns the selected index into `items`, or `-1` on cancellation without a
successful activation. Check its error. Zero Options gives an opaque, animated list;
the process font defaults to an installed Nerd Font, then bundled Go Mono.

Enter runs one callback on a separate goroutine and ignores further Enter presses
until it finishes. An error keeps the overlay open with a banner; nil closes it.
A nil callback selects immediately. Esc closes the surface, but `Run` waits for in-flight work.

Complete examples: [`dmenu`](example/dmenu) (list), [`wallpaper`](example/wallpaper) (grid).

## Grid layout

Set `Options.Grid` and each item's `Preview` path becomes a labelled tile.
Arrows move in two dimensions; typing fuzzy-filters. Bounded background workers
decode PNG/JPEG/GIF/WebP, letterboxed without cropping; pending tiles show placeholders.

![Thumbnail grid](../docs/media/menu-grid.png)

```sh
make -C menu wallpaper-demo                                # generated images
make -C menu wallpaper-demo WALLPAPER_DIR=~/Pictures/Walls  # your images
```

## How it works

[`layershell.go`](layershell.go) supplies the layer-shell binding; [`internal`](internal)
contains software rendering, input, matching and image workers. Theme colors come from
the XDG appearance portal over D-Bus, falling back to a built-in palette.

## Build

```sh
make check           # formatting, vet, tests in pinned Podman tooling
make vendor          # networked dependency refresh
make wallpaper-demo  # build and open the grid example
```

## Known limits

- US-QWERTY keymap only; no full xkb layout support.
- Callbacks cannot be cancelled. A callback that never returns also blocks `Run` forever;
  start long-lived processes without waiting for their exit, as the wallpaper example does.
- Thumbnail jobs have no prioritization/cancellation; fast scrolling can queue visible
  tiles behind previously drawn cells.

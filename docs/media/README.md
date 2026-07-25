# Screenshots

The images here are referenced from the project READMEs. They are captured **headlessly**, not
from anyone's desktop, so they can be regenerated on demand and never contain a stray window,
wallpaper, or hostname.

| File | Shows | Reproduce with |
| --- | --- | --- |
| `menu-grid.png` | the `menu` module's thumbnail grid, via the `wallpaper` example | `make -C menu wallpaper-demo` |
| `zlg-launcher.png` | the `zlg` launcher over the bundled demo apps | `make -C launcher/gui demo` |

Both were taken under a headless [sway](https://swaywm.org/) (`WLR_BACKENDS=headless`,
`WLR_RENDERER=pixman`) on a 1280x800 output, captured with `grim`. Sway rather than Weston
because the overlay is a `wlr-layer-shell` surface and Weston does not implement that protocol.

Two things the headless capture does not show, both expected:

- **No app icons in `zlg`.** The icon column is blank because the capture container has no
  freedesktop icon theme installed. Icon lookup is opportunistic by design - a missing icon
  leaves the slot empty rather than failing.
- **The system palette is the built-in fallback.** There is no XDG desktop portal in the
  container, so the theme resolver cannot read the desktop's light/dark preference or accent
  colour and falls back to the bundled palette. On a real desktop the overlay follows the
  system theme.

The wallpapers in the grid are generated, not photographs: see
[`menu/example/wallpaper/gen`](../../menu/example/wallpaper/gen). That keeps the repository free
of binary image assets and licensing questions, and makes the demo identical on every machine.

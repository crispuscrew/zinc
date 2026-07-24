# menu - a reusable Wayland overlay menu

`menu` is a floating, filterable overlay menu for Wayland, in pure Go. It opens a centered
`wlr-layer-shell` surface, software-renders a fuzzy-filtered list of items with a bundled
bitmap font, and feeds keyboard input into a picker model - a fuzzel/wofi-style panel that
floats above the tiled windows, not a tiled window itself. Give it a list and a callback, get
back the chosen item.

It was extracted out of the [`zlg`](../launcher/gui) launcher so it is not tied to Zinc:
`menu` depends on **no** Zinc sibling module (Go `replace` directives are not transitive, so
a sibling dependency would make it un-importable from another repo), only on the pure-Go /
cgo-free Wayland, D-Bus, and image libraries. So it builds **static, `CGO_ENABLED=0`**, and
is `go get`-able from anywhere - an app launcher, a wofi-like picker, or the
[`zde`](https://github.com/crispuscrew/zde) desktop's own menus.

## API

The whole public surface is one call plus three types (see [`menu.go`](menu.go)):

```go
func Run(items []Item, activate ActivateFunc, opts Options) (int, error)

type Item struct {
	Label       string // primary text, and what the fuzzy filter matches against
	Description string // secondary text, shown dimmed after the label
	Marked      bool   // draws an indicator dot; the caller decides what it means
}

type Options struct {
	Prompt  string  // drawn before the query (default "> ")
	AppID   string  // layer-surface namespace / app-id for compositor rules (default "menu")
	Width   int     // overlay width in px  (default 720)
	Height  int     // overlay height in px (default 440)
	Opacity float64 // background opacity 0..1; <= 0 means opaque
	NoAnim  bool    // disable the entrance fade-in
	Debug   bool    // trace the Wayland handshake to stderr
}

// Called on Enter. Returning an error keeps the menu open and shows it in a banner;
// returning nil closes the menu with that item selected.
type ActivateFunc func(item Item) error
```

`Run` returns the index of the activated item (into `items`), or `-1` if the user cancelled
(Esc, or the compositor closed the surface). The zero `Options` value is usable: a
default-size, opaque, animated overlay with a `"> "` prompt.

The `activate` callback is called on Enter **while the overlay is still up**, so a consumer
can do its work (launch a program, print a line) and, by returning an error, report a failure
in the window without tearing it down:

```go
package main

import (
	"fmt"

	"github.com/crispuscrew/zinc/menu"
)

func main() {
	items := []menu.Item{
		{Label: "firefox", Description: "Web browser"},
		{Label: "foot", Description: "Terminal", Marked: true},
	}

	activate := func(item menu.Item) error {
		// Do the work here. Return an error to keep the menu open and show it
		// in a banner; return nil to close the menu on this item.
		fmt.Println("chose", item.Label)
		return nil
	}

	index, err := menu.Run(items, activate, menu.Options{Prompt: "> ", AppID: "myapp"})
	if err != nil {
		panic(err)
	}
	if index < 0 {
		return // the user cancelled
	}
	// items[index] was activated.
}
```

## How it works

- **It speaks `wlr-layer-shell` directly.** go-wayland ships only the core protocol plus
  xdg-shell, so the `zwlr_layer_shell_v1` / `zwlr_layer_surface_v1` binding is **hand-written**
  in [`layershell.go`](layershell.go), in the same style as go-wayland's generated code. That
  is what lets the surface be a centered, keyboard-grabbing floating overlay.
- **It matches the system theme.** The palette is resolved from the XDG desktop portal
  (`org.freedesktop.appearance`: the dark/light preference and the accent color) over D-Bus
  through the pure-Go godbus client, and falls back to a built-in palette when no portal is
  reachable - so the menu looks like the rest of the desktop while staying cgo-free.
- **The core is pure and unit-tested.** Everything but the thin Wayland event loop lives in
  small internal packages: `internal/picker` (the fuzzy-filter view-model), `internal/keymap`
  (US-QWERTY key decoding), `internal/render` (the software renderer), `internal/theme` (the
  portal palette resolver), and `internal/match` (the fuzzy matcher, copied in rather than
  shared, to keep the no-sibling-dependency rule).

## Build

`menu` is a library, not a tool, so it only carries the containerized checks:

```sh
make check        # gofmt + vet + test in the pinned container
make vendor       # refresh vendored deps (the only step that needs network)
```

## Known limits

- The keymap is US-QWERTY; full keyboard-layout (xkb) support is future work.

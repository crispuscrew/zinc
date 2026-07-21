module github.com/crispuscrew/zinc/launcher/gui

go 1.24.2

toolchain go1.24.13

// zlg (zinc-launcher-gui) is the graphical sibling of zlt: the same quick picker over the
// defined apps, for a point-and-click / keyboard launch. Like the other tools it depends
// only on launcher/common (the shared app store, zcr delegate, and fuzzy matcher) and
// shells out to the `zcr` binary to run what it picks - it never imports the runner.
//
// zlg renders with pure Go: it speaks the Wayland wire protocol directly (go-wayland, no
// cgo) and software-renders the picker into a shared-memory buffer with a bundled bitmap
// font (golang.org/x/image). So it stays a static, CGO_ENABLED=0, runs-anywhere binary
// built from the same minimal image as the other tools - no graphics libraries, no dynamic
// linking. The picker view-model (internal/picker), the keymap (internal/keymap), and the
// renderer (internal/render) are pure and unit-tested; only the thin Wayland event loop
// (internal/ui) needs a live compositor.
//
// A consumer module carries the local replaces for every Zinc module in its graph
// (replaces are not transitive - only the main module's apply), so `go mod vendor`
// resolves launcher/common and common from the sibling tree and the build stays hermetic.
require (
	github.com/crispuscrew/zinc/launcher/common v0.0.0
	golang.org/x/image v0.18.0
)

require (
	github.com/crispuscrew/zinc/common v0.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/crispuscrew/zinc/launcher/common => ../common

replace github.com/crispuscrew/zinc/common => ../../common

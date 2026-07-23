module github.com/crispuscrew/zinc/launcher/gui

go 1.24.2

toolchain go1.24.13

// zlg (zinc-launcher-gui) is the graphical sibling of zlt: the same quick picker over the
// defined apps, for a point-and-click / keyboard launch. Like the other tools it depends on
// launcher/common (the shared app store and zcr delegate) and shells out to the `zcr` binary
// to run what it picks - it never imports the runner.
//
// The picker window itself is the reusable `menu` module (a pure-Go Wayland layer-shell
// overlay plus a software renderer); zlg is a thin consumer that supplies the app list and an
// activate callback. So zlg stays a static, CGO_ENABLED=0, runs-anywhere binary, and the
// Wayland / graphics / D-Bus deps come in transitively through menu.
//
// A consumer module carries the local replaces for every Zinc module in its graph (replaces
// are not transitive - only the main module's apply), so `go mod vendor` resolves menu,
// launcher/common, and common from the sibling tree and the build stays hermetic.
require (
	github.com/crispuscrew/zinc/launcher/common v0.0.0
	github.com/crispuscrew/zinc/menu v0.0.0
)

require (
	github.com/crispuscrew/zinc/common v0.0.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/rajveermalviya/go-wayland/wayland v0.0.0-20230130181619-0ad78d1310b2 // indirect
	golang.org/x/image v0.18.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/crispuscrew/zinc/menu => ../../menu

replace github.com/crispuscrew/zinc/launcher/common => ../common

replace github.com/crispuscrew/zinc/common => ../../common

// menu is the reusable, floating, filterable overlay-menu UI core: a pure-Go Wayland
// layer-shell surface plus a software renderer, extracted from the zlg launcher so any
// program can build its own menus over it (the zlg launcher, a wofi-like picker, the zde
// desktop's menus).
//
// It deliberately depends on NO Zinc sibling modules - Go `replace` directives are not
// transitive, so a sibling dep pinned at v0.0.0 would make menu un-importable from another
// repo. Its only requires are the pure-Go/cgo-free Wayland, D-Bus, and image libraries, so it
// builds static and `go get`-able from anywhere. (The fuzzy matcher is vendored in as
// internal/match rather than shared with launcher/common for the same reason.)
module github.com/crispuscrew/zinc/menu

go 1.24.2

toolchain go1.24.13

require (
	github.com/godbus/dbus/v5 v5.2.2
	github.com/rajveermalviya/go-wayland/wayland v0.0.0-20230130181619-0ad78d1310b2
	golang.org/x/image v0.18.0
)

require (
	golang.org/x/sys v0.27.0 // indirect
	golang.org/x/text v0.16.0 // indirect
)

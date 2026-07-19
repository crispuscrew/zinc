module github.com/crispuscrew/zinc/launcher/tui

go 1.24.2

toolchain go1.24.13

// zlt (zinc-launcher-tui) is a fast keyboard-driven picker over the defined apps. Like
// zcc it depends only on the shared library (schema + validate) and shells out to the
// `zcr` binary to run what it picks - it never imports the runner. The local replace
// keeps the per-module hermetic build: `go mod vendor` copies common's source into
// ./vendor, so the digest-pinned container build needs no network or sibling tree.
require github.com/crispuscrew/zinc/common v0.0.0

replace github.com/crispuscrew/zinc/common => ../../common

require (
	github.com/charmbracelet/bubbletea v1.3.10
	github.com/charmbracelet/lipgloss v1.1.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/colorprofile v0.4.1 // indirect
	github.com/charmbracelet/x/ansi v0.11.6 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.9.0 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.5.0 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.3.8 // indirect
)

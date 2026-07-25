module github.com/crispuscrew/zinc/launcher/common

go 1.24.2

toolchain go1.24.13

// launcher/common is the UI-agnostic core shared by the Zinc launchers (zlt today, zlg
// next): the read-side app store, the zcr delegate, and the fuzzy matcher. It depends
// only on the shared schema library (common) and never on a UI toolkit or the runner, so
// a TUI and a GUI front-end reuse ONE copy of the list / launch / match logic (and its
// security guards). The local replace keeps the per-module hermetic build: `go mod
// vendor` copies common's source into ./vendor, so the digest-pinned container build
// needs no network or sibling tree.
require (
	github.com/crispuscrew/zinc/common v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/crispuscrew/zinc/common => ../../common

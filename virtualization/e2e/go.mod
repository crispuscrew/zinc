// The VM end-to-end test module. It has no dependencies: the test is black-box, driving
// the built zc and zvr binaries (and qemu) via os/exec and asserting in Go, so it never
// imports the tools it exercises. It is guarded by the `e2e` build tag, so a plain
// `go test ./...` compiles nothing here; run it with `make e2e`.
module github.com/crispuscrew/zinc/virtualization/e2e

go 1.24.2

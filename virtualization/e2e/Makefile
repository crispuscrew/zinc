# VM end-to-end tests. They boot real guests under qemu, so unlike the per-tool `make
# check` they run on the HOST and need /dev/kvm plus a host `go` toolchain. Guarded by the
# `e2e` build tag, so an ordinary `go test` compiles nothing here. GOWORK=off because the
# dev workspace pins the sibling modules; this module is standalone.
#
# These are NOT a CI gate: GitHub's runners have no /dev/kvm, and an unaccelerated guest
# would take longer than the job budget. Run them locally before a release.

.PHONY: e2e clean
GO ?= go
TEST_ARGS ?=
## e2e: rebuild binaries (unless explicitly supplied), verify the image, and run scenarios
e2e:
	GOWORK=off $(GO) test -tags e2e -count=1 -timeout 900s -v $(TEST_ARGS) .

## clean: remove the downloaded guest image and any leftover state
clean:
	-rm -rf $${XDG_DATA_HOME:-$$HOME/.local/share}/zinc/vms/zinc-e2e-*

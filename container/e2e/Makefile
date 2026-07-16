# End-to-end tests. They drive the built zcc + zcr binaries against rootless podman, so
# unlike the per-tool `make check` they run on the HOST (the harness orchestrates real
# containers via the host's podman) and need a host `go` toolchain. Guarded by the `e2e`
# build tag, so an ordinary `go test` compiles nothing here. GOWORK=off because the dev
# workspace pins the sibling modules; this module is standalone.

.PHONY: e2e clean
## e2e: build what's missing (binaries, images) and run the end-to-end scenarios
e2e:
	GOWORK=off go test -tags e2e -count=1 -timeout 900s -v .

## clean: remove the test app image
clean:
	-podman rmi -f localhost/zinc/e2e-app:local

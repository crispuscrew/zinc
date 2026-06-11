# Shared build logic for every HyprZinc tool (hzp, hzl, hzv).
#
# "The same logic, only different paths": a tool's Makefile sets TOOL (and, if it
# has them, APP / RUN_ARGS), then `include ../tool.mk`. Everything below is
# identical across tools — the per-tool differences come from $(TOOL).
#
# The reproducible build itself lives in the repo-root ../Containerfile, which is
# generic (it builds whatever module is the build context). Run from a tool's
# module root: `make <target>`.

ifndef TOOL
$(error TOOL is not set — put `TOOL := hzp` above `include ../tool.mk`)
endif

GO             ?= go
CONTAINER_TOOL ?= podman
BIN_DIR        ?= bin
BIN            ?= $(BIN_DIR)/$(TOOL)

# Reproducible build image + the generic Containerfile shared by all tools.
BUILD_IMAGE    ?= hyprzinc/$(TOOL)-build:local
CONTAINERFILE  ?= ../Containerfile

# Extra args for `make run` (hzp sets `run <app>`); empty for the others.
RUN_ARGS       ?=

.DEFAULT_GOAL := help

## help: list available targets
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## //'

## build: build the binary — only inside the pinned container (alias for container-build)
build: container-build

## test: run unit tests
test:
	$(GO) test ./...

## vet: run go vet
vet:
	$(GO) vet ./...

## fmt: format sources in place
fmt:
	gofmt -w .

## fmt-check: fail if any file needs formatting
fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "needs gofmt:"; echo "$$unformatted"; exit 1; fi

## check: fmt-check + vet + test (the pre-commit gate, ROADMAP "every change")
check: fmt-check vet test

# --- Dependency / vendor maintenance (run by hand; vendor/update need network, the build never does) ---

## vendor: refresh vendored deps to match go.mod — tidy, vendor, verify
vendor:
	$(GO) mod tidy
	$(GO) mod vendor
	$(GO) mod verify

## vendor-check: fail if 'tidy + vendor' would change anything (dirty-tree safe; no-op for dep-free modules)
vendor-check:
	@snapshot="$$(mktemp -d)"; \
	cp go.mod "$$snapshot/go.mod"; \
	[ -f go.sum ] && cp go.sum "$$snapshot/go.sum" || true; \
	[ -d vendor ] && cp -a vendor "$$snapshot/vendor" || true; \
	$(GO) mod tidy && $(GO) mod vendor; \
	status=0; \
	diff -q "$$snapshot/go.mod" go.mod >/dev/null 2>&1 || status=1; \
	if [ -f go.sum ] || [ -f "$$snapshot/go.sum" ]; then diff -q "$$snapshot/go.sum" go.sum >/dev/null 2>&1 || status=1; fi; \
	if [ -d vendor ] || [ -d "$$snapshot/vendor" ]; then diff -rq "$$snapshot/vendor" vendor >/dev/null 2>&1 || status=1; fi; \
	rm -rf "$$snapshot"; \
	if [ $$status -ne 0 ]; then echo "vendor out of sync — run 'make vendor' and commit the result"; exit 1; fi; \
	echo "vendor-check: in sync"

## verify: check module checksums against go.sum (offline; integrity only)
verify:
	$(GO) mod verify

## update: upgrade dependencies (minor/patch, no major bumps), then re-vendor
update:
	$(GO) get -u ./...
	$(GO) mod tidy
	$(GO) mod vendor
	$(GO) mod verify

## run: run the tool from source (hzp passes its sample app; override with RUN_ARGS=...)
run:
	$(GO) run . $(RUN_ARGS)

## container-build: build reproducibly in the pinned container, extract to ./bin/<tool>
container-build:
	$(CONTAINER_TOOL) build -t $(BUILD_IMAGE) -f $(CONTAINERFILE) .
	@mkdir -p $(BIN_DIR)
	@cid=$$($(CONTAINER_TOOL) create $(BUILD_IMAGE)); \
	$(CONTAINER_TOOL) cp $$cid:/app $(BIN); \
	$(CONTAINER_TOOL) rm $$cid >/dev/null
	@echo "built $(BIN)"

## repro: build twice in-container and assert the binary is byte-identical
repro:
	$(MAKE) --no-print-directory container-build BIN=$(BIN_DIR)/$(TOOL).1
	$(MAKE) --no-print-directory container-build BIN=$(BIN_DIR)/$(TOOL).2
	@sha256sum $(BIN_DIR)/$(TOOL).1 $(BIN_DIR)/$(TOOL).2
	@cmp $(BIN_DIR)/$(TOOL).1 $(BIN_DIR)/$(TOOL).2 && echo "REPRODUCIBLE: identical bytes"

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)

.PHONY: help build test vet fmt fmt-check check vendor vendor-check verify update run container-build repro clean

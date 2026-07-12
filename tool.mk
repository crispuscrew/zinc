# Build logic for HyprZinc TOOLS (hzc, hzl, hzv) — the binary targets layered on
# top of the shared containerized checks in check.mk. A tool's Makefile sets TOOL
# (and APP / RUN_ARGS if it has them), then `include ../tool.mk`.
#
# Podman-only: the binary is produced by the reproducible repo-root ../Containerfile;
# there is no host `go` and no `go run`. Run from a tool's module root: `make <target>`.

ifndef TOOL
$(error TOOL is not set — put `TOOL := hzc` above `include ../tool.mk`)
endif

# Include check.mk relative to THIS file (not the invoking dir), so it resolves
# whatever depth the tool's Makefile sits at. check.mk defines REPO_REL, used below.
include $(dir $(lastword $(MAKEFILE_LIST)))check.mk

BIN_DIR        ?= bin
BIN            ?= $(BIN_DIR)/$(TOOL)

# Reproducible build image + the generic Containerfile shared by all tools (at the
# repo root, reached via REPO_REL so it is correct at any module depth).
BUILD_IMAGE    ?= hyprzinc/$(TOOL)-build:local
CONTAINERFILE  ?= $(REPO_REL)/Containerfile

# Extra args for `make run` (hzc sets `run <app>`); empty for the others.
RUN_ARGS       ?=

## build: build the binary reproducibly in the pinned container (alias for container-build)
build: container-build

## run: build in the pinned container, then run the produced binary (hzc drives the host podman; override args with RUN_ARGS=...)
run: build
	./$(BIN) $(RUN_ARGS)

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

.PHONY: build run container-build repro clean

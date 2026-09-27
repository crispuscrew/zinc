# Snapshot and vendor only under OUTPUT; repository vendor is never refreshed.
include ../../check.mk

ROOT := $(abspath ../..)
OUTPUT ?= /tmp/opencode/zinc-dns-check
SANDBOX = podman run --rm --userns=keep-id --user $$(id -u):$$(id -g) \
  --cap-drop=ALL --security-opt=no-new-privileges --security-opt=label=disable \
  -e GOTOOLCHAIN=local -e GOWORK=off -e GOCACHE=/scratch/cache -e GOPATH=/scratch/go
GO_RUN = $(SANDBOX) --network=none -v $(OUTPUT):/scratch -w /scratch/common $(GO_IMAGE)
GOFILES = find ./adapters/dnsproxy -name '*.go' -print

.PHONY: prepare lint build race
## prepare: copy common sources and create temporary vendor using pinned dependencies
prepare:
	mkdir -p $(OUTPUT)/common
	$(SANDBOX) -v $(ROOT)/common:/input:ro -v $(OUTPUT):/scratch -w /scratch/common $(GO_IMAGE) \
	  sh -c 'cp /input/go.mod /input/go.sum . && cp -R /input/domain /input/adapters . && go mod tidy && go mod vendor && go mod verify'

## lint: format owned sources and vet the temporary common module
lint: fmt-check vet

## build: compile the shared DNS adapter in the isolated snapshot
build:
	$(GO_RUN) go build ./adapters/dnsproxy

## race: run DNS race tests (requires a pinned image containing a C compiler)
race:
	$(GO_RUN) go test -race ./adapters/dnsproxy

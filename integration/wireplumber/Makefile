ROOT := $(abspath ../..)
OUTPUT ?= /tmp/opencode/zinc-audio-check
GO_IMAGE := docker.io/library/golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83
RUN = podman run --rm --network=none --cap-drop=ALL --security-opt=no-new-privileges --security-opt=label=disable \
  -e GOTOOLCHAIN=local -e GOWORK=off -e CGO_ENABLED=0
COMMON = -v $(ROOT)/common:/src:ro -w /src

.PHONY: help test lint build isolated
help:
	@printf '%s\n' 'test: isolated unit tests with canonical common sources' 'lint: vet and format check for owned Go sources' 'build: compile isolated integration test executable' 'isolated: run a private synthetic PipeWire/WirePlumber session'

test:
	$(RUN) $(COMMON) $(GO_IMAGE) go test ./domain/audio ./adapters/audio
	$(RUN) -v $(ROOT)/container/runner:/src:ro -w /src $(GO_IMAGE) go test ./adapters/pipewirectx

lint:
	$(RUN) $(COMMON) $(GO_IMAGE) go vet ./domain/audio ./adapters/audio
	$(RUN) -v $(ROOT)/container/runner:/src:ro -w /src $(GO_IMAGE) go vet ./adapters/pipewirectx
	$(RUN) $(COMMON) $(GO_IMAGE) sh -c 'test -z "$$(gofmt -l domain/audio adapters/audio)"'
	$(RUN) -v $(ROOT)/container/runner/adapters/pipewirectx:/src:ro -w /src $(GO_IMAGE) sh -c 'test -z "$$(gofmt -l .)"'

build:
	mkdir -p $(OUTPUT)
	$(RUN) $(COMMON) -v $(OUTPUT):/output $(GO_IMAGE) go test -c -o /output/audio.test ./adapters/audio

# This starts new private processes; it never addresses the user's session daemon.
isolated: build
	ZINC_AUDIO_TEST_POLICY=$(CURDIR) TMPDIR=$(OUTPUT) $(OUTPUT)/audio.test -test.v -test.run '^TestIsolated'

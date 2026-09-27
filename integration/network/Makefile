# Keep the compiler pin synchronized with ../../check.mk.
GO_IMAGE ?= docker.io/library/golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83
HELPER_IMAGE ?= sha256:$(shell podman image inspect --format '{{.Id}}' localhost/zinc/netfilter:local 2>/dev/null)

.DEFAULT_GOAL := help
.PHONY: help test lint

help:
	@printf '%s\n' 'test: real isolated packet tests (rootless Podman, no external uplink)' 'lint: format and vet checks in the pinned compiler'

test:
	GO_IMAGE='$(GO_IMAGE)' HELPER_IMAGE='$(HELPER_IMAGE)' TEST_PATTERN='$(TEST_PATTERN)' sh ./run.sh test

lint:
	GO_IMAGE='$(GO_IMAGE)' HELPER_IMAGE='$(HELPER_IMAGE)' sh ./run.sh lint

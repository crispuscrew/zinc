# Generic reproducible, hermetic build for any Zinc tool (zcc / zcr / ...).
#
# Goal: the produced binary must not depend on the host's Go version or environment -
# same inputs give the same bytes, on any machine (the "Stable" promise). Every input is
# pinned:
#   - the Go toolchain by DIGEST (below), refreshed deliberately, never floating
#   - dependencies by each module's ./vendor + go.sum (no proxy, no network)
#
# The build context is a tool's module dir; the shared Makefile invokes this as
#   podman build -f ../../Containerfile .
# so `make container-build` / `make repro` work identically in each tool module.
#
# Refresh the toolchain pin when you choose to upgrade:
#   podman pull docker.io/library/golang:1.24-alpine
#   podman image inspect --format '{{index .RepoDigests 0}}' golang:1.24-alpine
ARG GO_IMAGE=docker.io/library/golang:1.24-alpine@sha256:757779acac4af1b349a20f357c7296097b4a0b89da4ad0e370b339060077282a

FROM ${GO_IMAGE} AS build

# Never silently download a different toolchain - fail instead, so the pin holds.
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY . .

# Reproducible build flags:
#   CGO_ENABLED=0      static binary, independent of host libc
#   -mod=vendor        build only from ./vendor (used when the module has deps)
#   -trimpath          strip local filesystem paths from the binary
#   -buildvcs=false    don't embed VCS state (context has no .git anyway)
#   -ldflags=-buildid= drop the random build id; -s -w strip debug/symbol tables
#   -X main.version    stamp the release version (passed in by the Makefile); a plain
#                      build defaults to "dev". A tool without a `version` var ignores it.
# Dep-free modules (skeletons) have no ./vendor and build the same way without
# -mod=vendor - still hermetic, since there is nothing to fetch.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
RUN set -eu; \
    if [ -d vendor ]; then modflag=-mod=vendor; else modflag=; fi; \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
        go build $modflag -trimpath -buildvcs=false \
            -ldflags="-s -w -buildid= -X main.version=${VERSION}" -o /out/app .

# Carrier stage: a minimal image holding only the static binary, for extraction.
# ENTRYPOINT lets `podman create` succeed so `podman cp` can pull the binary out.
FROM scratch AS export
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]

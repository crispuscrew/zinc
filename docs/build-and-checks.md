# Builds, checks and toolchain compatibility

Repository tool builds use rootless Podman and the shared `Containerfile`,
`check.mk` and `tool.mk`. The current exact Go image is:

```text
docker.io/library/golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83
```

`common/go.mod` and `go.work` declare Go 1.26.0. The build/check toolchain is
Go 1.26.6; host end-to-end harnesses also need a supported Go version >=1.26.6.
Go 1.24/1.25 is insufficient for the current common dependency graph, notably
quic-go. `GOTOOLCHAIN=local` prevents silent toolchain downloads in pinned builds.

## Module commands

```sh
make -C common help
make -C common check
make -C creator check build
make -C container/runner check build
make -C virtualization/runner check build
make -C launcher/common check
make -C launcher/tui check build
make -C launcher/gui check build
make -C menu check
```

`check` is `fmt-check`, `vet`, `test`; it does not rewrite formatting. `fmt`
does rewrite source. During the canonical-schema transition, preserve the
owner-controlled `common/domain/schema/schema.go` formatting and report any
format-gate failure rather than changing that file incidentally.

`common` and `launcher/common` are libraries; they have no tool binary build
target. Tool `build` writes `bin/<name>`; `repro` builds twice and compares bytes.
Compilation uses vendored dependencies, no network, cgo disabled, trimpath,
no VCS stamping and a cleared build ID. Version text is stamped from
`git describe`, falling back to `dev` when unavailable.

These guarantees assume synchronized inputs. Each consumer has its own vendor
tree; editing canonical common code does not update those copies. `go.work` is
for local development, not the isolated per-module build input.

## Vendor maintenance

Canonical common includes DNS/QUIC dependencies. Its vendor and all six consumer
vendor trees are regenerated through Make. After shared source changes, refresh
the affected consumers before checking their builds; stale vendored code does
not establish compatibility with canonical sources.

The explicit maintenance operation is `make vendor` per affected module
(`GOWORK=off`: tidy, vendor, verify). It is networked and changes module files
and vendor; consumers' Go directives will be lifted as required by common.
Review that refresh separately. `vendor-check` also runs tidy/vendor and is not
a read-only substitute when preserving a working tree.

For migration verification, use a temporary snapshot/module overlay against
canonical sources with pinned dependencies and read-only repository mounts.
Keep caches and temporary vendor outside the checkout. The DNS integration
Makefile demonstrates this under `/tmp/opencode`; report overlay checks as such,
not as proof that repository vendor is current.

## Examples and integration

`common/examples` strictly decodes all canonical YAML with known-field checking
and a single-document check, then runs shared validation. The broken fixture
asserts its intended errors. JSON examples validate DNS transport descriptions
and VM option/app bindings. No example test creates topology, boots a guest or
contacts external DNS. To include launcher fixtures in a module-only container
check, mount them read-only and set `ZINC_DEMO_APPS` to that path.

Runtime suites have separate prerequisites:

- [Container e2e](../container/e2e/README.md): real Podman and binaries; network
  scenarios require externally provisioned fixtures.
- [VM e2e](../virtualization/e2e/README.md): QEMU/KVM and guest images; offline
  lifecycle smoke and separately gated provisioned-network scenarios.
- [WirePlumber](../integration/wireplumber/README.md): private daemons and
  synthetic devices for policy/permission/revocation checks.
- [DNS](../integration/dns/README.md): transport, parsing, readiness and lifecycle
  tests on private sockets; race checks need the pinned C-compiler image.

CI gates module checks/builds, vendor consistency, reproducibility, container
e2e and the Nix consumer path. Its existing setup-go action installs 1.26.6 for
the host harness. Hosted CI does not establish local KVM or desktop-policy
compatibility; prerequisite-gated skips are not evidence of enforcement.

## Nix and releases

The flake is a separate consumer build path pinned by `flake.lock`, not the
repository's authoritative toolchain gate. All five tools select
`pkgs.buildGo126Module`. The unchanged locked nixpkgs provides Go 1.26.5,
which satisfies the modules' Go 1.26 requirement. Both Linux architectures
evaluate; all five x86_64-linux binaries were built using this compiler.

The Nix `runtime-integration` package contains deployment material under
`share/zinc`. The home-manager module installs selected binaries; neither it nor
the deployment package configures Podman, QEMU, networking or audio services.
`release.mk` manages
signed tags and has no Go image pin. Historical release binaries remain tied
to their tagged schema; do not use them to test new canonical examples.

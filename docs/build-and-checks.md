# Builds, checks and toolchain compatibility

Rootless Podman [build](../Containerfile)/[check](../check.mk)/[tool](../tool.mk) pipeline, pinned Go:

```text
docker.io/library/golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83
```

`common/go.mod`/`go.work`: Go 1.26.0; build/check: 1.26.6; host e2e: supported Go >=1.26.6.
Go 1.24/1.25 cannot build current common/quic-go. `GOTOOLCHAIN=local` forbids toolchain downloads.

## Module commands

Run within each module (`make help` lists targets):

| Modules | Command |
| --- | --- |
| `common`, `launcher/common`, `menu` | `make check`; libraries, no tool binary build |
| `creator`, `container/runner`, `virtualization/runner`, `launcher/tui`, `launcher/gui` | `make check build` |

`check` = read-only `fmt-check`, `vet`, `test`; `fmt` rewrites source. Report schema
format-gate failures without incidentally editing owner-controlled `common/domain/schema/schema.go`.
`build` writes `bin/<name>`; `repro` compares two builds. Compilation uses offline vendor,
no cgo, trimpath, no VCS stamping, cleared build ID; version: `git describe`, fallback `dev`.

## Vendor maintenance

Refresh common's DNS/QUIC vendor and affected copies among six consumers after shared edits;
stale vendors cannot prove compatibility. `go.work` is development-only, not build input.
`make vendor` per affected module uses `GOWORK=off` tidy/vendor/verify: networked,
mutates module/vendor files and may lift consumer Go directives. Review separately.
`vendor-check` also runs tidy/vendor; it is not read-only.

For migration, use [pinned canonical-source overlays](../integration/dns/README.md#pinned-checks)
with read-only repo mounts and external caches/vendor (`/tmp/opencode`).
Report overlay results, not repository vendor freshness.

## Examples and integration

`common/examples` checks known fields/single YAML document/shared validation, intended
broken-fixture errors, DNS JSON and VM option/app binding. It creates no topology,
guest or external DNS traffic. Mount launcher fixtures read-only; set `ZINC_DEMO_APPS`.

| Suite | Separate prerequisites / scope |
| --- | --- |
| [Container e2e](../container/e2e/README.md) | Real Podman/binaries; externally provisioned network fixtures |
| [VM e2e](../virtualization/e2e/README.md) | QEMU/KVM; blank-disk offline lifecycle and gated guest-image/network fixtures |
| [WirePlumber](../integration/wireplumber/README.md#checks) | Private daemons, synthetic policy/permission/revocation tests |
| [DNS](../integration/dns/README.md#pinned-checks) | Private-socket transport/parsing/readiness/lifecycle; race requires pinned C-compiler image |

CI gates checks/builds/vendors/reproducibility/container e2e/Nix; setup-go installs
1.26.6 for host tests. Skips prove no enforcement; hosted CI cannot certify local KVM/desktop policy.

## Nix and releases

`flake.lock` pins a consumer path, not the authoritative gate: all five tools use
`pkgs.buildGo126Module`; locked Go 1.26.5 satisfies module 1.26.
`runtime-integration` packages deployment files under `share/zinc`; home-manager installs
selected binaries. Neither configures Podman/QEMU/network/audio services.
`release.mk` manages signed tags, no Go image pin; tagged binaries cannot test newer schemas.

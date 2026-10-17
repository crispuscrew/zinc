# Zinc - Release Plan

**0.11.1** adds the guided installer and streamlined schema v4 documentation.
[Changelog](CHANGELOG.md) records release details;
[GitHub Releases](https://github.com/crispuscrew/zinc/releases) records publication and assets.

| Version | Milestone |
| --- | --- |
| 0.1.0 | Containers: `zc` + `zcr` MVP |
| 0.2.0 | Terminal launcher: `zlt` MVP |
| 0.3.0 | Graphical launcher: `zlg` MVP |
| 0.4.0 | Virtualization: `zvr` MVP, VM authoring in `zc` |
| 0.5.0 | Guest GPU: Venus Vulkan + verified virgl OpenGL |
| 0.6.0 | Windows guests: UEFI/Secure Boot/TPM, install, identity, display/drivers |
| 0.7.0 | Containment: resources/users, routing/WireGuard, readiness, inheritance, domains, Compose |
| 0.8.0 | Filtered D-Bus, Apache 2.0, pinned CI runtime |
| 0.8.1 | Nix/home-manager, instance addressing, `zcr where` |
| 0.8.2 | Instance launches/state mounts, pin recheck, `zc init` |
| 0.9.0 | Wayland security contexts, bus attribution, network counters/posture |
| 0.9.1 | 22 audit fixes, including injection, relaunch and firewall defects |
| 0.10.0 | Schema v3: audio, configs/volumes, notifications, env/rootfs/display controls, VM egress, signed tags/checksums |
| 0.10.1 | Verified Linux AMD64 binaries, quickstart, Node.js 24 Actions |
| 0.11.0 | Schema v4, provisioned networking, encrypted DNS, audio broker, external VM options, Go 1.26 |
| 0.11.1 | Verified installer, guided offline example, concise documentation and packaged references |

Schema-breaking pre-1.0 changes require a minor bump: 0.10.0 introduced v3;
**0.11.0 is a breaking pre-1.0 minor release**, changing definitions and runtime prerequisites.
Read its [release notes](docs/releases/0.11.0.md) and [migration guide](docs/migration.md).

## Cutting a release

For the next release, merge feature PRs into `dev`, cut a fresh `release/X.Y.Z`
from `dev`, then PR to `main`.
Protected-branch PRs require review and green CI; create a signed, annotated tag on the final reviewed commit:

```sh
make -f release.mk tag VERSION=X.Y.Z
```

Requires `git config user.signingkey` (`gpg.format=ssh` for SSH); no unsigned fallback.
Pushing the tag triggers pinned builds, version verification and CI-generated `SHA256SUMS`;
CI gates publication on `make repro` for every tool. Linux AMD64 releases stay draft until
all assets are attached and verified. Local candidate checksums are not publication evidence.
Keep `flake.nix`, the `.github/workflows/ci.yml` version assertion, the installer's
default version, this table and `docs/quickstart.md` synchronized; finalize `CHANGELOG.md`.

ZDE (`zde-niri` / `zde-hypr`) has a separate repository/release plan; this repo releases Zinc core/tools.

# Zinc - Roadmap

Current contracts: [architecture](docs/architecture.md).
Milestones and publication policy: [release plan](RELEASES.md); history: [Changelog](CHANGELOG.md).

## Current milestone - 0.11.0

**0.11.0** aligns schema v4 consumers, examples, vendors and canonical formatting.
Both runners expose DNS workers with
authenticated readiness; networking and WirePlumber still require explicit host deployment.
See [release notes and verification scope](docs/releases/0.11.0.md) and [migration](docs/migration.md).
Publication follows reviewed `dev -> release/0.11.0 -> main` PRs, green CI and a signed tag.

Earlier milestones below are historical summaries; their implementation details and
measured results remain in the Changelog. Current behavior is defined by the focused references.

## 0.1 - Containers (zc + zcr) - done

`zc`/`zcr`, schema validation, rootless lifecycle, image pins and network policy ([history](CHANGELOG.md#010---2026-07-16)).

## 0.2 - Launcher TUI (zlt) - done

Keyboard-first fuzzy app picker and runner delegation ([history](CHANGELOG.md#020---2026-07-19)).

## 0.3 - Launcher GUI (zlg) - done

Wayland picker, shared launcher logic and reusable menu/grid ([history](CHANGELOG.md#030---2026-07-25)).

## 0.4 - Virtualization (zvr) - done

Direct QEMU, pinned bases/overlays, cloud-init and shared authoring ([history](CHANGELOG.md#040---2026-07-26)).

## 0.5 - Guest GPU access - done

Measured virgl OpenGL and opt-in Venus Vulkan, with sandbox tradeoffs ([history](CHANGELOG.md#050---2026-07-26)).

## 0.6 - Windows-class guests - done

UEFI/Secure Boot/TPM, installation, machine identity and display/driver support ([history](CHANGELOG.md#060---2026-07-27)).

## Beyond the version line - container hardening

0.7-0.10.1 delivered containment/routing, filtered D-Bus, Nix packaging, instances,
attestation, audit fixes, schema v3 and verified distribution; see [release history](CHANGELOG.md).
Schema v4 replaces automatic network topology with [owner provisioning](docs/network-provisioning.md).

Open items, without assigned release dates:

- **Live domain refresh:** rules remain launch-time [IP snapshots](docs/network-policy.md#domains-are-ip-snapshots).
- **Curated images:** locally built, digest-pinned trusted bases beyond per-app install layers.
- **Key agents:** SSH/GPG agent integration beyond current [protected key mounts](docs/images-and-mounts.md#keys-and-themes).
- **Declarative desktop setup:** app seeds and desktop wiring beyond the shipped Nix packages,
  home-manager tool selection and `zc init`; session profiles, hotkeys and login autostart.
- **UI:** full xkb keymaps, launch cancellation and thumbnail prioritization/cancellation;
  see [menu limits](menu/README.md#known-limits).
- **VM management limits:** no snapshots/managed save or guest agent; guest commands,
  environment/filesystem/bus bridges and graphical Background remain unavailable.
  See [VM boundaries](docs/virtualization.md) and [Windows/GPU limits](docs/vm-hardware.md).

### Cross-cutting

- Functional core, adapter-owned I/O; follow [contributor rules](CONTRIBUTING.md).
- Changes need tests and `make check` in touched modules; releases need runnable exit checks.
- Report partial mechanisms and skipped checks honestly; limits live in the focused references.
- ZDE (`zde-niri` / `zde-hypr`) has its own repository, milestones and release plan.

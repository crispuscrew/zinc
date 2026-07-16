# Zinc - Roadmap

Source of truth: [`docs/architecture.md`](docs/architecture.md). Release plan:
[`RELEASES.md`](RELEASES.md). Priority order: **Stable, then Secure, then Beautiful**.

**Style:** functional core / imperative shell. The schema, validation, and the podman
argv/ruleset builders are pure functions over decoded data; I/O and process execution live
at the edges (the adapters, the CLI/TUI). Every release ships with tests and a runnable
exit check.

Legend: done, in progress, planned.

---

## 0.1 - Containers (zcc + zcr) - in progress

The two container tools reach MVP.

Delivered:
- The v2 app-config **schema** and pure **validation** in `common/` (including the rule
  that third-party images must be digest-pinned).
- A YAML config store under `~/.config/zinc/apps`, with save-time and launch-time
  validation.
- **zcr**: real rootless-container lifecycle (`run`/`stop`/`restart`/`inspect`/`logs`/
  `ps`), derived images (`FROM image` + the install layer), multiterminal apps, and a
  `--version` stamped from git.
- **zcc**: a keyboard-first Bubbletea TUI plus a scriptable CLI; it authors app files and
  shells out to `zcr` to run them.
- **Network lock-down**, applied in the app's own netns by an nftables init step before the
  app starts, with no unfiltered window: isolated (localhost only), egress whitelist,
  ingress publish to the LAN, and per-port sibling links over a private bridge.
- Podman-only reproducible builds, an end-to-end suite against real podman, and CI.

Known gaps (honest, tracked): the network model still rejects (does not run) host-scoped
egress, gateway / multi-homing, and combining a sibling link with other networking on one
app; bundle-relative config mounts are deferred. Test coverage is partial away from the
security path. `launcher/` and `virtualization/creator/` do not yet compile (they still
reference the removed `core` module).

## 0.2 - Launcher TUI (zlt) - planned

A fast, keyboard-driven picker (TUI) over the defined apps: fuzzy search, launch through
`zcr`, dependency auto-start.

## 0.3 - 0.6 - Zinc Desktop Environment (zde-niri) - planned

The `zde-niri` variant reaches MVP (0.3), gains automatic qemu config for testing (0.4),
then a visual helper (0.6). A GUI launcher (`zlg`) lands at 0.5.

## 0.7 - Virtualization (zvc + zvr) - planned

VM apps as the container tools' sibling: a creator and a runner over rootless
libvirt/qemu, sharing the same config library and format.

---

## Beyond the version line - container hardening

Container work that matures alongside the releases above, not tied to one version:

- **vpn-container routing:** an app routed through a sibling VPN app with per-destination
  backend selection and fail-closed DNS (the network model already has the directional,
  fail-closed foundation; this extends it and lifts the "combining a link with other
  networking" restriction).
- **Trusted image layering:** curated, digest-pinned base images built locally, so an app
  can reference a known-good base without a hand-written Containerfile.
- **Theme bundle, audio, keys, mounts:** a read-only theme bundle + env for host-matching
  GTK/Qt apps; pipewire / legacy-ALSA audio on explicit grant; ssh/gpg key mounts with
  agent sockets and 0600 enforcement; general host mounts.
- **Nix home-manager module + flake:** the tools on `$PATH`, a first-run seed of app files,
  and desktop wiring, all reproducible.
- **Profiles, hotkeys, autostart:** named session profiles, desktop hotkeys, and login
  autostart.

---

### Cross-cutting

- **Honesty:** where a mechanism is partial, say so in the UI and the docs.
- **Every change:** `make check` green (gofmt + vet + test) in every module you touched.
- Known tradeoffs are tracked in the architecture doc.

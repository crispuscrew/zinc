# Zinc - Roadmap

Source of truth: [`docs/architecture.md`](docs/architecture.md). Release plan:
[`RELEASES.md`](RELEASES.md). Priority order: **Stable, then Secure, then Beautiful**.

**Style:** functional core / imperative shell. The schema, validation, and the podman
argv/ruleset builders are pure functions over decoded data; I/O and process execution live
at the edges (the adapters, the CLI/TUI). Every release ships with tests and a runnable
exit check.

Legend: done, in progress, planned.

---

## 0.1 - Containers (zc + zcr) - done

The two container tools reach MVP.

Delivered:
- The v2 app-config **schema** and pure **validation** in `common/` (including the rule
  that third-party images must be digest-pinned).
- A YAML config store under `~/.config/zinc/apps`, with save-time and launch-time
  validation.
- **zcr**: real rootless-container lifecycle (`run`/`stop`/`restart`/`inspect`/`logs`/
  `ps`), derived images (`FROM image` + the install layer), multiterminal apps, and a
  `--version` stamped from git.
- **zc**: a keyboard-first Bubbletea TUI plus a scriptable CLI; it authors app files and
  shells out to `zcr` to run them.
- **Network lock-down**, applied in the app's own netns by an nftables init step before the
  app starts, with no unfiltered window: isolated (localhost only), egress whitelist,
  ingress publish to the LAN, and per-port sibling links over a private bridge.
- Podman-only reproducible builds, an end-to-end suite against real podman, and CI.

Known gaps (honest, tracked): the network model still rejects (does not run) host-scoped
egress, gateway / multi-homing, and combining a sibling link with other networking on one
app; bundle-relative config mounts are deferred. Test coverage is partial away from the
security path. (At 0.1 `launcher/` and `virtualization/creator/` did not compile either - they
still referenced the removed `core` module. `launcher/` was rebuilt in 0.2 and 0.3; the
virtualization skeleton was deleted in 0.4, when `zc` took over authoring both app types.)

## 0.2 - Launcher TUI (zlt) - done

A fast, keyboard-driven picker (TUI) over the defined apps: fuzzy filter as you type,
enter launches the selected app through `zcr` (which handles dependency auto-start), a
running indicator from `zcr ps`, and a `zlt <app>` direct-launch form for a desktop
hotkey. Like `zc` it depends only on `common` and shells out to the `zcr` binary, so it
never imports the runtime. Lives at `launcher/tui`, leaving `launcher/gui` for `zlg`.

## 0.3 - Launcher GUI (zlg) - done

A graphical sibling to `zlt`: the same quick picker over the defined apps, for a
point-and-click launch. Like the other tools it depends only on `common` and shells out to
the `zcr` binary, so it never imports the runtime.

Delivered:
- **zlg** at `launcher/gui`: a floating `wlr-layer-shell` overlay picker - fuzzy filter as you
  type, a dot for apps already running, `zlg <app>` for a hotkey - as a static, `CGO_ENABLED=0`,
  byte-reproducible binary with no cgo and no graphics libraries.
- **menu** (`menu/`, its own module): the overlay core extracted so `zde` and a future
  wofi-like tool can import it - a hand-written layer-shell binding, a software renderer, a
  keymap, a theme resolver and a picker view-model behind one `menu.Run` call, depending on no
  sibling module. Its `ActivateFunc` runs off the event loop (0.3.1), so a slow launch leaves
  the overlay responsive instead of freezing it with the keyboard grabbed.
- **launcher/common**: the read-side app store, the `zcr` delegate and the fuzzy matcher, so
  `zlt` and `zlg` share one copy of the list / launch / match logic and its security guards.
- **Richer rows**: app grouping (schema v2's additive `Group` field), freedesktop icon lookup,
  antialiased text in an auto-detected system Nerd Font, and a thumbnail **grid** layout with
  bounded background decoding, shown off by the `wallpaper` example.

Known gaps: the keymap is a fixed US-QWERTY fallback rather than a real xkb interpreter; a
launch cannot be cancelled once started (Esc dismisses the overlay, the launch continues); and
grid thumbnails are decoded without prioritisation, so fast scrolling queues visible tiles
behind ones already scrolled past.

## 0.4 - Virtualization (zvr) - done

VM apps as the container tools' sibling, sharing the same config library and format: one
store holds both kinds, split by `Type`, and each runner refuses the other's apps by name.

Delivered:
- **zvr** at `virtualization/runner`: boots VM apps as qemu guests. Copy-on-write overlays
  over a digest-pinned base (`zvr reset` returns an app to its authored image), a cloud-init
  seed for first-boot identity, supervision by pidfile and QMP, and a graceful stop that
  presses the guest's ACPI power button rather than killing it mid-write.
- **Schema**: `Type: ZincVirtualization` plus a `VirtualizationMeta` group. The two kinds
  reject each other's fields rather than ignoring them.
- **zcc renamed to zc** (`zinc-creator`), moved to `creator/`, and taught to author both
  kinds. There is no separate `zvc`; the old `virtualization/creator` skeleton was deleted.

Deliberate departure from the original plan: **qemu is driven directly, not through
libvirt.** libvirtd spawns the qemu process outside the user's session, so it cannot open an
accelerated window on their compositor, and SPICE-plus-viewer adds a hop that defeats the
point for anything interactive. The cost, accepted knowingly, is that zvr owns supervision
and has no snapshots or managed save. See the architecture doc, Virtualization.

Also delivered: an end-to-end suite against real qemu (`virtualization/e2e`, a local gate
rather than a CI one - GitHub's runners have no `/dev/kvm`), and `make -C
virtualization/runner demo`, which fetches a Fedora Cloud image, verifies it against
Fedora's own published digest and boots it.

**GPU status, measured rather than assumed.** Guest **OpenGL is hardware-accelerated**: a
Fedora guest reports `virgl (NVIDIA GeForce RTX 5080/PCIe/SSE2)` at GL 4.2 core, so guest GL
really is running on the host GPU. Guest **Vulkan is not** - it measures as `llvmpipe`, the
CPU. 0.4 therefore ships as a genuinely OpenGL-accelerated VM runner with software Vulkan;
closing the Vulkan gap is 0.5 (below).

## 0.5 - Guest GPU access - planned

Make a guest's 3D actually reach the GPU, which is what turns a VM app from an isolation
tool into somewhere a real workload can run.

- **Vulkan through venus** - the whole of 0.5, and the half that matters for games, since
  Proton, DXVK and vkd3d are all Vulkan.

  Investigated on 2026-07-26, and further than it first looked. Fedora 43's virglrenderer
  1.2.0 has no venus support, so `venus=on` fails; building virglrenderer 1.2.0 with
  `-Dvenus=true` into a private prefix and pointing qemu at it with `LD_LIBRARY_PATH` gets
  the library loaded (confirmed in `/proc/<pid>/maps`) but venus **still** fails to
  initialize. The failure is in `proxy_renderer_init`: venus does not run in qemu's process
  at all, it runs in a separate `virgl_render_server` process that the library forks and
  execs (overridable with `RENDER_SERVER_EXEC_PATH`). The exec is not the problem - no exec
  error is logged and the server binary runs standalone - so the handshake between the two
  is where it stops.

  The most likely root cause to check next: venus requires the host Vulkan driver to export
  dma-buf (`vkr_physical_device.c` probes `is_dma_buf_fd_export_supported`), and NVIDIA's
  proprietary driver has long been weak there. If that is it, venus may be unreachable on
  this hardware regardless of build flags, and the honest answer becomes "OpenGL guests
  here, Vulkan needs different hardware or a Mesa-driven GPU".

  Whatever the outcome, venus becomes opt-in per app rather than a default: enabling it
  where the host cannot support it costs the guest its display entirely.
- **A measurement target**, so the answer is a number rather than an impression, and a
  regression is visible.

Honest boundary on this hardware: full GPU passthrough is not an option here at all. A
single RTX 5080 with no integrated GPU means passing it through blinds the host.

**ZDE** (the Zinc Desktop Environment, `zde-niri` / `zde-hypr`) is a separate project in
its own repository, layered on these tools; its milestones are tracked there, not in this
plan. This repo ships only the Zinc core and its tools.

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

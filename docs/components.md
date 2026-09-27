# Components and host integration

| Binary | Role | Implementation boundary |
| --- | --- | --- |
| `zc` | Author apps, manage through CLI/TUI | Shared common library, subprocess runner delegation |
| `zcr` | Container launch/lifecycle | Rootless Podman and desktop/network adapters |
| `zvr` | VM launch/supervision | Direct QEMU, disk/firmware and shared broker adapters |
| `zlt` | Terminal app picker | Shared launcher store/matcher/delegate, Bubbletea |
| `zlg` | Graphical app picker | Shared launcher logic and reusable Wayland menu |

The creator authors both app types. It imports no runner and carries no runtime;
run/manage commands cross a process boundary to the runner chosen by `Type`.
Authoring works without either binary installed. Backend limitations are surfaced
instead of pretending all commands have equivalent VM/container behavior.

App definitions live under `$XDG_CONFIG_HOME/zinc/apps`; launcher presentation
comes from `LauncherMeta`. A bare name resolves in the store; explicit YAML paths
are also supported. Runtime state and per-instance paths are separate from
authored app bundles and VM hardware options.

## Creator and conversion

`zc` combines a store, runner delegate, backend facade, Bubbletea forms, keybind
schemes and Compose conversion. Default/vim/custom schemes configure the TUI,
not desktop hotkeys; custom schemes live under `~/.config/zinc/zc`.
Advanced YAML editing handles complete network/mount structures. Forms preserve
explicit values and detect concurrent changes; inherited apps need sparse file
editing rather than form saves that would flatten intent.

Compose conversion reports losses. Export cannot carry ordered packet
enforcement, broker isolation or every lifecycle/raw-flag/audio setting. Import
does not invent outbound permission or raw privilege flags; only representable
explicit networking is translated. Published ports that cannot retain their
binding/translation semantics are reported rather than silently broadened.
Dependencies express ordering, not connectivity or readiness. See
[creator README](../creator/README.md) for current flags.

## Launchers and menu

Both pickers support fuzzy selection and direct `zlt APP`/`zlg APP` launch. They
delegate by app type and use running-state information from the relevant runner.
They do not duplicate validation or enforcement.

`launcher/common` contains store, matching and process-boundary code. `zlt` uses
Bubbletea. `zlg` uses the independent `menu` module: a pure-Go, cgo-free
`wlr-layer-shell` overlay with software shared-memory rendering and portal-based
theme selection. Missing icons/palette services have presentation fallbacks.
Layer-shell support is required; a generic Wayland session is not sufficient.
The menu keymap remains limited; see [menu README](../menu/README.md).

`menu.Run(items, activate, opts)` is reusable without importing a Zinc sibling
module, avoiding transitive local-replace assumptions for outside consumers.
Bundled picker fixtures under `launcher/demo/apps` are canonical offline apps;
opening a picker is different from provisioning or launching those apps.

## Repository and dependency direction

- `common/domain`: pure schema, migration, inheritance, validation, resolved
  network policy, audio plans and VM options.
- `common/adapters`: shared network, DNS and audio I/O. The common module is no
  longer wholly pure or dependent only on YAML; QUIC/DNS add pinned dependencies.
- `container/runner/domain`, `ports`, `app`, `adapters`, `wire`: inward-facing
  policy/contracts, orchestration, concrete runtime/broker mechanisms, composition.
- `virtualization/runner`: QEMU argv policy, disk/firmware/process adapters and
  supervisor orchestration, sharing common contracts rather than a container pod.
- `creator`: authoring/UI/conversion; `launcher/{common,tui,gui}`: picker layers.
- `container/e2e`, `virtualization/e2e`: actual-runtime tests with prerequisites.
- `integration/{wireplumber,dns}`: shared adapter checks and deployment contracts.
- Root `Containerfile`, `check.mk`, `tool.mk`: shared pinned build/check pipeline.

Mechanisms belong in adapters; domain code does not open files, read environment
or invoke Podman/nft/QEMU. The network port remains a replaceable mechanism, but
its contract now requires packet-preserving provisioned identity.

## Host surface

Host prerequisites depend on grants: rootless Podman, a suitable compositor and
terminal for containers; QEMU/KVM, image/firmware tools and optional swtpm for VMs;
explicit network provisioning for NICs; opt-in WirePlumber policy for PipeWire.
Zinc has per-instance holders/supervisors rather than a central daemon, and does
not automatically install host services or change host firewall rules.

Important environment inputs include `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
`XDG_RUNTIME_DIR`, `WAYLAND_DISPLAY`, `ZINC_TERMINAL`/`TERMINAL`,
`ZINC_THEME_BUNDLE`, `ZINC_NETFILTER_IMAGE`, and `ZINC_NETWORK_MANIFEST_DIR`.
Host adapters translate them into explicit launch options.

ZDE is a separate desktop-integration project. Session profiles, desktop hotkeys,
theme-bundle production and compositor policy belong there or to the host owner,
not to schema validation. Zinc can be adopted in parts; installing its binaries
does not establish every runtime prerequisite.

# Components and host integration

| Binary | Role | Implementation boundary |
| --- | --- | --- |
| `zc` | Author apps, manage through CLI/TUI | Shared common library, subprocess runner delegation |
| `zcr` | Container launch/lifecycle | Rootless Podman and desktop/network adapters |
| `zvr` | VM launch/supervision | Direct QEMU, disk/firmware and shared broker adapters |
| `zlt` | Terminal app picker | Shared launcher store/matcher/delegate, Bubbletea |
| `zlg` | Graphical app picker | Shared launcher logic and reusable Wayland menu |

`zc` authors either type without installed runners; run/manage delegates by `Type`
and reports backend limitations. App names resolve under `$XDG_CONFIG_HOME/zinc/apps`;
explicit YAML paths also work. `LauncherMeta` controls presentation. Runtime and
instance state stay separate from authored bundles and VM options.

## Creator and conversion

- Default/vim/custom keybinds affect only its TUI; custom files: `~/.config/zinc/zc`.
- Forms preserve explicit values and detect concurrent edits. Network/mount structures
  use advanced YAML; inherited apps require sparse file editing instead of form saves.
- [Compose conversion](../creator/README.md#editing-and-conversion) reports losses:
  export loses ordered enforcement, broker isolation and some lifecycle/raw/audio settings.
  Import invents neither outbound grants nor raw privileges; unrepresentable port
  bindings/translations are reported, never broadened. Dependencies only order startup.

## Launchers and menu

Both support fuzzy selection and `zlt APP`/`zlg APP`; runners supply execution,
running state, validation and enforcement. `launcher/common` owns store/matching/delegation.
`zlt` uses Bubbletea; `zlg` requires `wlr-layer-shell`, not just generic Wayland.

The independent, cgo-free [menu](../menu/README.md) uses software shared-memory
rendering, portal themes and icon/palette fallbacks. Its keymap is limited.
`menu.Run(items, activate, opts)` needs no Zinc sibling/transitive local replacements.
Opening a picker neither provisions nor launches the offline `launcher/demo/apps` fixtures.

## Repository and dependency direction

| Path | Responsibility |
| --- | --- |
| `common/domain` | Pure schema, migration, inheritance, validation, network/audio plans, VM options |
| `common/adapters` | Network/DNS/audio I/O; common now includes pinned DNS/QUIC dependencies |
| `container/runner/{domain,ports,app,adapters,wire}` | Policy/contracts, orchestration, mechanisms, composition |
| `virtualization/runner` | QEMU policy, disk/firmware/process adapters, supervision; shared contracts, no container pod |
| `creator`, `launcher/{common,tui,gui}` | Store/delegate/backend facade, Bubbletea forms, conversion; picker layers |
| `{container,virtualization}/e2e`, `integration/{wireplumber,dns}` | Runtime tests, adapter checks/deployment contracts |
| `Containerfile`, `check.mk`, `tool.mk` | Pinned build/check pipeline |

Domains do no I/O; replaceable network adapters must preserve provisioned packet identity.

## Host surface

Grants require rootless Podman/compositor/terminal, or QEMU/KVM/image/firmware tools
and optional swtpm; NICs need provisioning, PipeWire needs deployed policy.
Components can be adopted separately; their holders/supervisors install no host services/firewall.

Adapters consume `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_RUNTIME_DIR`, `WAYLAND_DISPLAY`,
`ZINC_TERMINAL`/`TERMINAL`, `ZINC_THEME_BUNDLE`, `ZINC_NETFILTER_IMAGE`,
`ZINC_NETWORK_MANIFEST_DIR`. ZDE/host owners supply session profiles, desktop hotkeys,
theme bundles and compositor policy.

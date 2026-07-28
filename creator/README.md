# zc - Zinc Container Creator

`zc` authors Zinc app files and manages them. It writes app definitions to
`~/.config/zinc/apps/<name>.yaml` and knows nothing about podman: to actually run what it
authors, it shells out to the `zcr` binary (the Zinc container runtime). The two meet
only at the on-disk YAML format and at that process boundary.

`zcr` must be on your `$PATH` for the run/manage commands. Authoring (new, edit, list,
validate) works without it.

## Commands

Authoring (local, no runtime needed):

```
zc tui                             keyboard-first manager (create/edit/run/stop/logs)
zc new <name> --image <img> [--desc d] [--icon i] [--tunnel wg.conf]
zc list
zc validate <name|app.yaml> [--resolved]       --resolved prints what an inheriting app merges to
zc delete <name>
zc keys list|show|set <s>|edit|validate|path   TUI keybind schemes
zc compose export <name> [-o f]                describe an app as a Compose-spec file
zc compose import <compose.yaml> [--service s] [--dry-run]
```

Both compose directions are lossy and print exactly how: exporting cannot carry the egress
lock-down, and importing invents no network access.

Runtime (forwarded verbatim to `zcr`):

```
zc run <name|app.yaml> [--exec]    build the launch plan; print it, or launch
zc build <name|app.yaml>           (re)build the app's derived image
zc stop|restart|inspect <name>
zc logs <name> [-f]
zc term <name> [--shell]           open a terminal for a multiterminal app
zc image search <term>|resolve <ref>
```

A bare `<name>` resolves against the store (`~/.config/zinc/apps`); an argument that
looks like a path (contains `/` or ends in `.yaml`) is read directly.

## Build

Podman-only, reproducible in a pinned container:

```
make build      # produces ./bin/zc
make check      # gofmt + vet + test, in-container
```

Put both `zc` and `zcr` on your `$PATH` to author and run apps.

## Layout

- `main.go` - the CLI: authoring is handled locally; runtime commands are forwarded to `zcr`.
- `internal/store` - the YAML app store (`~/.config/zinc/apps`).
- `internal/runner` - the `zcr` delegate (finds `zcr` on `$PATH` and drives it).
- `internal/backend` - the one facade the CLI and TUI use (store for authoring, `zcr` for running).
- `internal/tui` - the keyboard-first terminal UI.
- `internal/keys` - the TUI keybind schemes (`~/.config/zinc/zc`).

It depends only on the shared `common` library (schema + validation); it never imports
the runner.

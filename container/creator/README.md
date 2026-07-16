# zcc - Zinc Container Creator

`zcc` authors Zinc app files and manages them. It writes app definitions to
`~/.config/zinc/apps/<name>.yaml` and knows nothing about podman: to actually run what it
authors, it shells out to the `zcr` binary (the Zinc container runtime). The two meet
only at the on-disk YAML format and at that process boundary.

`zcr` must be on your `$PATH` for the run/manage commands. Authoring (new, edit, list,
validate) works without it.

## Commands

Authoring (local, no runtime needed):

```
zcc tui                             keyboard-first manager (create/edit/run/stop/logs)
zcc new <name> --image <img> [--desc d] [--icon i]
zcc list
zcc validate <name|app.yaml>
zcc delete <name>
zcc keys list|show|set <s>|edit|validate|path   TUI keybind schemes
```

Runtime (forwarded verbatim to `zcr`):

```
zcc run <name|app.yaml> [--exec]    build the launch plan; print it, or launch
zcc build <name|app.yaml>           (re)build the app's derived image
zcc stop|restart|inspect <name>
zcc logs <name> [-f]
zcc term <name> [--shell]           open a terminal for a multiterminal app
zcc image search <term>|resolve <ref>
```

A bare `<name>` resolves against the store (`~/.config/zinc/apps`); an argument that
looks like a path (contains `/` or ends in `.yaml`) is read directly.

## Build

Podman-only, reproducible in a pinned container:

```
make build      # produces ./bin/zcc
make check      # gofmt + vet + test, in-container
```

Put both `zcc` and `zcr` on your `$PATH` to author and run apps.

## Layout

- `main.go` - the CLI: authoring is handled locally; runtime commands are forwarded to `zcr`.
- `internal/store` - the YAML app store (`~/.config/zinc/apps`).
- `internal/runner` - the `zcr` delegate (finds `zcr` on `$PATH` and drives it).
- `internal/backend` - the one facade the CLI and TUI use (store for authoring, `zcr` for running).
- `internal/tui` - the keyboard-first terminal UI.
- `internal/keys` - the TUI keybind schemes (`~/.config/zinc/zcc`).

It depends only on the shared `common` library (schema + validation); it never imports
the runner.

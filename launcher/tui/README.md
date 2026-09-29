# zlt - Zinc launcher (TUI)

`zlt` fuzzy-picks apps from `$XDG_CONFIG_HOME/zinc/apps` (default `~/.config`).
It delegates launch to `zcr` for containers or `zvr` for VMs.

## Use

```sh
zlt            # open the picker
zlt <app>      # launch directly from a hotkey or script
zlt --version
```

Type to filter; Up/Down or Ctrl+P/Ctrl+N move, Ctrl+U clears, Enter launches and
quits, Esc/Ctrl+C cancel. A dot marks running apps (best effort, from both runners).

Launch runs `zcr run <app> --exec` or `zvr run <app>`; the selected binary must be
on `PATH`. Listing works without runners. See [runtime prerequisites](../../docs/quickstart.md).

## Build

```sh
make check   # formatting, vet, tests in pinned Podman tooling
make build   # bin/zlt
make vendor  # networked dependency refresh
```

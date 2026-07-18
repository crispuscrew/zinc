# zlt - Zinc launcher (TUI)

`zlt` (zinc-launcher-tui) is a fast, keyboard-first fuzzy picker over the apps you
defined with `zcc`. It lists everything in `~/.config/zinc/apps`, filters as you type,
and launches the chosen app by shelling out to the `zcr` binary - so, like `zcc`, it
depends only on the shared schema library and never imports the runtime.

## Use

```sh
zlt            # open the picker
zlt <app>      # launch a defined app directly (for a desktop hotkey or a script)
zlt --version
```

In the picker: **type** to fuzzy-filter, **up/down** (or **ctrl+p/ctrl+n**) to move,
**ctrl+u** to clear the filter, **enter** to launch the selected app (then it quits,
dmenu-style), **esc**/**ctrl+c** to cancel. A `●` marks apps that are already running
(best-effort, from `zcr ps`).

Launching runs `zcr run <app> --exec`, so `zcr` does the real work: validation,
dependency auto-start, the derived-image build, and the fail-closed network lock-down.
`zcr` must be on your `$PATH` to launch (the picker still lists apps without it).

## Build

Podman-only, reproducible, like the other tools:

```sh
make check     # gofmt + vet + test in the pinned container
make build     # reproducible build -> ./bin/zlt
make vendor    # refresh vendored deps (the only networked step)
```

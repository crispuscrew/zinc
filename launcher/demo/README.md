# Zinc launcher demo apps

Try the bundled app definitions from the repository root:

```sh
make -C launcher/gui demo     # the GUI picker (zlg)
make -C launcher/tui demo     # the TUI picker (zlt)
```

Targets build the launcher and set `XDG_CONFIG_HOME` to its throwaway `bin/demo-home`,
preserving your real config. Type to filter, arrows/Ctrl+N/Ctrl+P move, Enter launches,
Esc quits. See [GUI](../gui/README.md) or [TUI](../tui/README.md) requirements.

Launching needs `zcr` on `PATH` and rootless Podman: it builds/runs digest-pinned
Alpine plus `apk add`. Pull the exact base digest first; package installation needs
repository access. Firefox is graphical; others need a terminal. To keep a definition,
copy it to `~/.config/zinc/apps/` after reviewing the [examples](../../common/examples/README.md).
